package grpc

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/AS203038/looking-glass/pkg/bmp"
	"github.com/AS203038/looking-glass/pkg/errs"
	"github.com/AS203038/looking-glass/pkg/logging"
	"github.com/AS203038/looking-glass/pkg/routers/parse"
	"github.com/AS203038/looking-glass/pkg/utils"
	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"github.com/AS203038/looking-glass/protobuf/lookingglass/v0/lookingglassconnect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

// rpcLog is the component-tagged logger used by every RPC handler.
var rpcLog = logging.Component("rpc")

// rpcLogTag builds the slog attributes describing one RPC invocation.
func rpcLogTag(op string, routerID int64, ri *utils.RouterInstance) []slog.Attr {
	attrs := []slog.Attr{
		slog.String("op", op),
		slog.Int64("router_id", routerID),
	}
	if ri != nil && ri.Config != nil {
		attrs = append(attrs,
			slog.String("router", ri.Config.Name),
			slog.String("type", ri.Config.Type))
	}
	return attrs
}

// logRPCError emits one structured WARN event describing a failed RPC.
func logRPCError(tag []slog.Attr, stage string, err error) {
	attrs := make([]slog.Attr, 0, len(tag)+2)
	attrs = append(attrs, tag...)
	attrs = append(attrs, slog.String("stage", stage), slog.Any("err", err))
	rpcLog.LogAttrs(context.Background(), slog.LevelWarn, "rpc failed", attrs...)
}

// logRPCStart emits a DEBUG event at the top of a successful RPC handler.
func logRPCStart(tag []slog.Attr) {
	rpcLog.LogAttrs(context.Background(), slog.LevelDebug, "rpc start", tag...)
}

// logRPCOK emits an INFO event summarising a successful RPC.
func logRPCOK(tag []slog.Attr, start time.Time, raw []byte, pr parse.Result) {
	attrs := make([]slog.Attr, 0, len(tag)+4)
	attrs = append(attrs, tag...)
	attrs = append(attrs,
		slog.Duration("duration", time.Since(start)),
		slog.Int("result_bytes", len(raw)),
		slog.String("parser_kind", pr.Kind.String()),
		slog.String("parse_status", pr.Status.String()))
	rpcLog.LogAttrs(context.Background(), slog.LevelInfo, "rpc ok", attrs...)
}

// logRPCMetaStart emits a DEBUG event at the top of a metadata RPC
// handler (one with no router context, like GetInfo or GetRouters).
func logRPCMetaStart(op string, extra ...slog.Attr) {
	attrs := make([]slog.Attr, 0, len(extra)+1)
	attrs = append(attrs, slog.String("op", op))
	attrs = append(attrs, extra...)
	rpcLog.LogAttrs(context.Background(), slog.LevelDebug, "rpc start", attrs...)
}

// logRPCMetaOK emits an INFO event summarising a successful metadata
// RPC. Metadata RPCs have no parser kind / status, so those fields are
// deliberately absent here.
func logRPCMetaOK(op string, start time.Time, extra ...slog.Attr) {
	attrs := make([]slog.Attr, 0, len(extra)+2)
	attrs = append(attrs, slog.String("op", op),
		slog.Duration("duration", time.Since(start)))
	attrs = append(attrs, extra...)
	rpcLog.LogAttrs(context.Background(), slog.LevelInfo, "rpc ok", attrs...)
}

// runParser invokes the vendor-template-declared parser for op against
// raw; falls back to [parse.Disabled] when the router exposes no parser.
func runParser(ri *utils.RouterInstance, op parse.Op, raw []byte) parse.Result {
	type parserAware interface {
		Parser(op string) (parse.Parser, parse.Config)
	}
	pa, ok := ri.Router.(parserAware)
	if !ok {
		return parse.Disabled()
	}
	p, cfg := pa.Parser(string(op))
	return p.Parse(op, raw, cfg)
}

// joinSSH concatenates per-command stdout strings into the wire `result` bytes.
func joinSSH(parts []string) []byte {
	return []byte(strings.Join(parts, "\n"))
}

// nowPB returns the current server time as a *timestamppb.Timestamp.
func nowPB() *timestamppb.Timestamp {
	ts := time.Now()
	return &timestamppb.Timestamp{
		Seconds: ts.Unix(),
		Nanos:   int32(ts.Nanosecond()),
	}
}

// LookingGlassService is the ConnectRPC implementation of the
// LookingGlassService protobuf service.
type LookingGlassService struct {
	lookingglassconnect.UnimplementedLookingGlassServiceHandler
	// ctx is the server-lifetime context.
	ctx context.Context
	// rts is the immutable router catalogue populated at startup.
	rts utils.RouterMap
}

var _ lookingglassconnect.LookingGlassServiceHandler = (*LookingGlassService)(nil)

// NewLookingGlassService constructs the [LookingGlassService] handler
// bound to ctx and the supplied router catalogue.
func NewLookingGlassService(ctx context.Context, rts utils.RouterMap) lookingglassconnect.LookingGlassServiceHandler {
	return &LookingGlassService{
		ctx: ctx,
		rts: rts,
	}
}

// GetInfo returns the server's hostname and version string.
func (s *LookingGlassService) GetInfo(ctx context.Context, req *connect.Request[emptypb.Empty]) (*connect.Response[pb.GetInfoResponse], error) {
	start := time.Now()
	logRPCMetaStart("GetInfo")
	h, err := os.Hostname()
	if err != nil {
		h = "unknown"
	}
	version := utils.Version()
	logRPCMetaOK("GetInfo", start,
		slog.String("hostname", h),
		slog.String("version", version))
	return connect.NewResponse(&pb.GetInfoResponse{
		Hostname: h,
		Version:  version,
	}), nil
}

// GetRouters returns one page of the router catalogue. Defaults: limit=10, page=1.
func (s *LookingGlassService) GetRouters(ctx context.Context, req *connect.Request[pb.GetRoutersRequest]) (*connect.Response[pb.GetRoutersResponse], error) {
	tStart := time.Now()
	var ret []*pb.Router
	lim := req.Msg.GetLimit()
	page := req.Msg.GetPageToken()
	total := uint32(len(s.rts))
	logRPCMetaStart("GetRouters",
		slog.Uint64("limit", uint64(lim)),
		slog.Uint64("page", uint64(page)),
		slog.Uint64("total", uint64(total)))
	if lim == 0 {
		lim = 10
	}
	if page == 0 {
		page = 1
	}
	start := (page - 1) * lim
	end := start + lim
	if end > total {
		end = total
	}
	if start > total {
		logRPCMetaOK("GetRouters", tStart,
			slog.Int("returned", 0),
			slog.Uint64("total", uint64(total)),
			slog.Uint64("next_page", 0))
		return connect.NewResponse(&pb.GetRoutersResponse{}), nil
	}
	for k, v := range s.rts[start:end] {
		ret = append(ret, &pb.Router{
			Name:     v.Config.Name,
			Location: v.Config.Location,
			Id:       int64(start) + int64(k) + 1,
			Health: &pb.RouterHealth{
				Healthy: v.HealthCheck.Healthy,
				Timestamp: &timestamppb.Timestamp{
					Seconds: v.HealthCheck.Checked.Unix(),
					Nanos:   int32(v.HealthCheck.Checked.Nanosecond()),
				},
			},
			BmpActive: bmp.IsBMPActive(ctx, rpcCacheClient, v.Config.Name),
		})
	}
	var nextPage uint32
	if end < total {
		np := page + 1
		nextPage = np
	}
	logRPCMetaOK("GetRouters", tStart,
		slog.Int("returned", len(ret)),
		slog.Uint64("total", uint64(total)),
		slog.Uint64("next_page", uint64(nextPage)))
	return connect.NewResponse(&pb.GetRoutersResponse{
		Routers:  ret,
		NextPage: nextPage,
	}), nil
}

// Ping executes the ping command sequence against the requested router.
func (s *LookingGlassService) Ping(ctx context.Context, req *connect.Request[pb.PingRequest]) (*connect.Response[pb.PingResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		logRPCError(rpcLogTag("Ping", rt, nil), "router_lookup", errs.UnknownRouter)
		return nil, errs.UnknownRouter
	}
	tag := rpcLogTag("Ping", rt, ri)
	start := time.Now()
	logRPCStart(tag)
	target, err := utils.NewIPNetFromProtobuf(req.Msg.GetTarget())
	if err != nil {
		logRPCError(tag, "parse_target", err)
		return nil, errs.IPInvalid
	}
	key := pingCacheKey(rt, target.String())
	var cached pb.PingResponse
	if rpcCacheGet(ctx, key, &cached) {
		out := connect.NewResponse(&cached)
		out.Header().Set("X-Cache", "HIT")
		logRPCOK(tag, start, cached.GetResult(), parse.Result{Kind: cached.GetParserKind(), Status: cached.GetParseStatus()})
		return out, nil
	}
	ret, err := ri.Ping(target)
	if err != nil {
		logRPCError(tag, "ping_exec", err)
		if errors.Is(err, errs.PoolExhausted) {
			return nil, connect.NewError(connect.CodeResourceExhausted, err)
		}
		return nil, errs.ExecFailed
	}
	raw := joinSSH(ret)
	pr := runParser(ri, parse.OpPing, raw)
	resp := &pb.PingResponse{
		Result:      raw,
		Timestamp:   nowPB(),
		ParserKind:  pr.Kind,
		ParseStatus: pr.Status,
	}
	if ps, ok := pr.Payload.(*pb.PingStats); ok {
		resp.Parsed = ps
	}
	rpcCacheSet(ctx, key, resp)
	logRPCOK(tag, start, raw, pr)
	return connect.NewResponse(resp), nil
}

// Traceroute executes the traceroute command sequence against the requested router.
func (s *LookingGlassService) Traceroute(ctx context.Context, req *connect.Request[pb.TracerouteRequest]) (*connect.Response[pb.TracerouteResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		logRPCError(rpcLogTag("Traceroute", rt, nil), "router_lookup", errs.UnknownRouter)
		return nil, errs.UnknownRouter
	}
	tag := rpcLogTag("Traceroute", rt, ri)
	start := time.Now()
	logRPCStart(tag)
	target, err := utils.NewIPNetFromProtobuf(req.Msg.GetTarget())
	if err != nil {
		logRPCError(tag, "parse_target", err)
		return nil, errs.IPInvalid
	}
	key := tracerouteCacheKey(rt, target.String())
	var cached pb.TracerouteResponse
	if rpcCacheGet(ctx, key, &cached) {
		out := connect.NewResponse(&cached)
		out.Header().Set("X-Cache", "HIT")
		logRPCOK(tag, start, cached.GetResult(), parse.Result{Kind: cached.GetParserKind(), Status: cached.GetParseStatus()})
		return out, nil
	}
	ret, err := ri.Traceroute(target)
	if err != nil {
		logRPCError(tag, "traceroute_exec", err)
		if errors.Is(err, errs.PoolExhausted) {
			return nil, connect.NewError(connect.CodeResourceExhausted, err)
		}
		return nil, errs.ExecFailed
	}
	raw := joinSSH(ret)
	pr := runParser(ri, parse.OpTraceroute, raw)
	resp := &pb.TracerouteResponse{
		Result:      raw,
		Timestamp:   nowPB(),
		ParserKind:  pr.Kind,
		ParseStatus: pr.Status,
	}
	if tp, ok := pr.Payload.(*pb.TracerouteParsed); ok {
		resp.Parsed = tp
	}
	rpcCacheSet(ctx, key, resp)
	logRPCOK(tag, start, raw, pr)
	return connect.NewResponse(resp), nil
}

// BGPSummary executes the neighbour-summary command sequence against the requested router.
func (s *LookingGlassService) BGPSummary(ctx context.Context, req *connect.Request[pb.BGPSummaryRequest]) (*connect.Response[pb.BGPSummaryResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		logRPCError(rpcLogTag("BGPSummary", rt, nil), "router_lookup", errs.UnknownRouter)
		return nil, errs.UnknownRouter
	}
	tag := rpcLogTag("BGPSummary", rt, ri)
	start := time.Now()
	logRPCStart(tag)
	key := bgpSummaryCacheKey(rt)
	var cached pb.BGPSummaryResponse
	if rpcCacheGet(ctx, key, &cached) {
		out := connect.NewResponse(&cached)
		out.Header().Set("X-Cache", "HIT")
		logRPCOK(tag, start, cached.GetResult(), parse.Result{Kind: cached.GetParserKind(), Status: cached.GetParseStatus()})
		return out, nil
	}

	// Try BMP first
	if bmp.IsBMPActive(ctx, rpcCacheClient, ri.Config.Name) {
		peers, err := bmp.GetBMPPeers(ctx, rpcCacheClient, ri.Config.Name)
		if err != nil {
			logRPCError(tag, "bgp_summary_bmp_get", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("BMP datastore lookup failed"))
		}
		var pbPeers []*pb.BGPPeer
		for _, p := range peers {
			pbPeers = append(pbPeers, &pb.BGPPeer{
				PeerIp:           p.IP,
				PeerAsn:          p.ASN,
				State:            p.State,
				UptimeSeconds:    p.UptimeSeconds,
				PrefixesReceived: p.PrefixesReceived,
				PrefixesAccepted: p.PrefixesAccepted,
			})
		}
		resp := &pb.BGPSummaryResponse{
			Result:    []byte("Served from stateless BMP Redis store\n"),
			Timestamp: nowPB(),
			Parsed: &pb.BGPSummaryParsed{
				Peers: pbPeers,
			},
			ParserKind:  pb.ParserKind_PARSER_KIND_BUILTIN,
			ParseStatus: pb.ParseStatus_PARSE_STATUS_OK,
		}
		rpcCacheSet(ctx, key, resp)
		logRPCOK(tag, start, resp.Result, parse.Result{Kind: pb.ParserKind_PARSER_KIND_BUILTIN, Status: pb.ParseStatus_PARSE_STATUS_OK})
		return connect.NewResponse(resp), nil
	}

	ret, err := ri.BGPSummary()
	if err != nil {
		logRPCError(tag, "bgp_summary_exec", err)
		if errors.Is(err, errs.PoolExhausted) {
			return nil, connect.NewError(connect.CodeResourceExhausted, err)
		}
		return nil, errs.ExecFailed
	}
	raw := joinSSH(ret)
	pr := runParser(ri, parse.OpBGPSummary, raw)
	resp := &pb.BGPSummaryResponse{
		Result:      raw,
		Timestamp:   nowPB(),
		ParserKind:  pr.Kind,
		ParseStatus: pr.Status,
	}
	if bs, ok := pr.Payload.(*pb.BGPSummaryParsed); ok {
		resp.Parsed = bs
	}
	rpcCacheSet(ctx, key, resp)
	logRPCOK(tag, start, raw, pr)
	return connect.NewResponse(resp), nil
}

// BGPRoute executes the bgp.route command sequence against the requested router.
func (s *LookingGlassService) BGPRoute(ctx context.Context, req *connect.Request[pb.BGPRouteRequest]) (*connect.Response[pb.BGPRouteResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		logRPCError(rpcLogTag("BGPRoute", rt, nil), "router_lookup", errs.UnknownRouter)
		return nil, errs.UnknownRouter
	}
	tag := rpcLogTag("BGPRoute", rt, ri)
	start := time.Now()
	logRPCStart(tag)
	target, err := utils.NewIPNetFromProtobuf(req.Msg.GetTarget())
	if err != nil {
		logRPCError(tag, "parse_target", err)
		return nil, errs.IPInvalid
	}
	key := bgpRouteCacheKey(rt, target.String())
	var cached pb.BGPRouteResponse
	if rpcCacheGet(ctx, key, &cached) {
		out := connect.NewResponse(&cached)
		out.Header().Set("X-Cache", "HIT")
		logRPCOK(tag, start, cached.GetResult(), parse.Result{Kind: cached.GetParserKind(), Status: cached.GetParseStatus()})
		return out, nil
	}

	// Try BMP first
	if bmp.IsBMPActive(ctx, rpcCacheClient, ri.Config.Name) {
		routes, err := bmp.FindBMPRoutesByPrefix(ctx, rpcCacheClient, ri.Config.Name, target.String())
		if err != nil {
			logRPCError(tag, "bgp_route_bmp_get", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("BMP datastore lookup failed"))
		}
		var pbPaths []*pb.BGPPath
		for _, r := range routes {
			pbPaths = append(pbPaths, &pb.BGPPath{
				Prefix:           r.Prefix,
				AsPath:           r.ASPath,
				Nexthop:          r.NextHop,
				Communities:      r.Communities,
				LargeCommunities: r.LargeCommunities,
			})
		}
		resp := &pb.BGPRouteResponse{
			Result:    []byte("Served from stateless BMP Redis store\n"),
			Timestamp: nowPB(),
			Parsed: &pb.BGPPaths{
				Paths: pbPaths,
			},
			ParserKind:  pb.ParserKind_PARSER_KIND_BUILTIN,
			ParseStatus: pb.ParseStatus_PARSE_STATUS_OK,
		}
		rpcCacheSet(ctx, key, resp)
		logRPCOK(tag, start, resp.Result, parse.Result{Kind: pb.ParserKind_PARSER_KIND_BUILTIN, Status: pb.ParseStatus_PARSE_STATUS_OK})
		return connect.NewResponse(resp), nil
	}

	ret, err := ri.BGPRoute(target)
	if err != nil {
		logRPCError(tag, "bgp_route_exec", err)
		if errors.Is(err, errs.PoolExhausted) {
			return nil, connect.NewError(connect.CodeResourceExhausted, err)
		}
		return nil, errs.ExecFailed
	}
	raw := joinSSH(ret)
	pr := runParser(ri, parse.OpBGPRoute, raw)
	resp := &pb.BGPRouteResponse{
		Result:      raw,
		Timestamp:   nowPB(),
		ParserKind:  pr.Kind,
		ParseStatus: pr.Status,
	}
	if bp, ok := pr.Payload.(*pb.BGPPaths); ok {
		resp.Parsed = bp
	}
	rpcCacheSet(ctx, key, resp)
	logRPCOK(tag, start, raw, pr)
	return connect.NewResponse(resp), nil
}

// BGPPeerRoutes executes peer-specific routing lookups.
func (s *LookingGlassService) BGPPeerRoutes(ctx context.Context, req *connect.Request[pb.BGPPeerRoutesRequest]) (*connect.Response[pb.BGPPeerRoutesResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		logRPCError(rpcLogTag("BGPPeerRoutes", rt, nil), "router_lookup", errs.UnknownRouter)
		return nil, errs.UnknownRouter
	}
	tag := rpcLogTag("BGPPeerRoutes", rt, ri)
	start := time.Now()
	logRPCStart(tag)

	peerIP := req.Msg.GetPeerIp()
	peerName := req.Msg.GetPeerName()

	// Sanitize inputs
	var err error
	if peerName != "" {
		peerName, err = utils.SanitizeBGPPeerName(peerName)
		if err != nil {
			logRPCError(tag, "sanitize_peer_name", err)
			return nil, err
		}
	}
	if peerIP != "" {
		if net.ParseIP(peerIP) == nil {
			logRPCError(tag, "sanitize_peer_ip", errs.IPInvalid)
			return nil, errs.IPInvalid
		}
	}

	qTypeStr := ""
	switch req.Msg.GetQueryType() {
	case pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_RECEIVED:
		qTypeStr = "received"
	case pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_ACCEPTED:
		qTypeStr = "accepted"
	case pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_REJECTED:
		qTypeStr = "rejected"
	case pb.PeerRouteQueryType_PEER_ROUTE_QUERY_TYPE_ADVERTISED:
		qTypeStr = "advertised"
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("unsupported query type"))
	}

	limit := int64(req.Msg.GetLimit())
	if limit <= 0 {
		limit = 50
	}
	var cursor uint64
	if req.Msg.GetPageToken() != "" {
		c, err := strconv.ParseUint(req.Msg.GetPageToken(), 10, 64)
		if err == nil {
			cursor = c
		}
	}

	key := rpcCacheKeyPrefix() + "peerroutes:" + strconv.FormatInt(rt, 10) + ":" + peerIP + ":" + peerName + ":" + qTypeStr + ":" + strconv.FormatUint(cursor, 10) + ":" + strconv.FormatInt(limit, 10)
	var cached pb.BGPPeerRoutesResponse
	if rpcCacheGet(ctx, key, &cached) {
		out := connect.NewResponse(&cached)
		out.Header().Set("X-Cache", "HIT")
		logRPCOK(tag, start, cached.GetResult(), parse.Result{Kind: cached.GetParserKind(), Status: cached.GetParseStatus()})
		return out, nil
	}

	// Try BMP first
	if bmp.IsBMPActive(ctx, rpcCacheClient, ri.Config.Name) {
		resolvedIP := peerIP
		if resolvedIP == "" && peerName != "" {
			peers, err := bmp.GetBMPPeers(ctx, rpcCacheClient, ri.Config.Name)
			if err == nil {
				for _, p := range peers {
					if p.IP == peerName || p.BGPID == peerName {
						resolvedIP = p.IP
						break
					}
				}
			}
		}

		if resolvedIP == "" {
			logRPCError(tag, "peer_routes_bmp_no_ip", errors.New("unresolved peer IP under BMP mode"))
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("peer IP must be provided or resolvable under BMP mode"))
		}

		routes, err := bmp.GetBMPPeerRoutes(ctx, rpcCacheClient, ri.Config.Name, resolvedIP, qTypeStr)
		if err != nil {
			logRPCError(tag, "peer_routes_bmp_get", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("BMP datastore lookup failed"))
		}

		var pbPaths []*pb.BGPPath
		for _, r := range routes {
			pbPaths = append(pbPaths, &pb.BGPPath{
				Prefix:           r.Prefix,
				AsPath:           r.ASPath,
				Nexthop:          r.NextHop,
				Communities:      r.Communities,
				LargeCommunities: r.LargeCommunities,
			})
		}
		resp := &pb.BGPPeerRoutesResponse{
			Result:    []byte("Served from stateless BMP Redis store\n"),
			Timestamp: nowPB(),
			Parsed: &pb.BGPPaths{
				Paths: pbPaths,
			},
			ParserKind:    pb.ParserKind_PARSER_KIND_BUILTIN,
			ParseStatus:   pb.ParseStatus_PARSE_STATUS_OK,
			NextPageToken: "",
		}
		rpcCacheSet(ctx, key, resp)
		logRPCOK(tag, start, resp.Result, parse.Result{Kind: pb.ParserKind_PARSER_KIND_BUILTIN, Status: pb.ParseStatus_PARSE_STATUS_OK})
		return connect.NewResponse(resp), nil
	}

	// Fallback to SSH Template execution
	ret, err := ri.BGPPeerRoutes(peerIP, peerName, qTypeStr)
	if err != nil {
		logRPCError(tag, "peer_routes_exec", err)
		if errors.Is(err, errs.PoolExhausted) {
			return nil, connect.NewError(connect.CodeResourceExhausted, err)
		}
		return nil, errs.ExecFailed
	}
	raw := joinSSH(ret)
	opName := parse.OpBGPPeerRoutesReceived
	switch qTypeStr {
	case "accepted":
		opName = parse.OpBGPPeerRoutesAccepted
	case "rejected":
		opName = parse.OpBGPPeerRoutesRejected
	case "advertised":
		opName = parse.OpBGPPeerRoutesAdvertised
	}

	pr := runParser(ri, opName, raw)
	resp := &pb.BGPPeerRoutesResponse{
		Result:      raw,
		Timestamp:   nowPB(),
		ParserKind:  pr.Kind,
		ParseStatus: pr.Status,
	}
	if bp, ok := pr.Payload.(*pb.BGPPaths); ok {
		resp.Parsed = bp
	}
	rpcCacheSet(ctx, key, resp)
	logRPCOK(tag, start, raw, pr)
	return connect.NewResponse(resp), nil
}

// BGPCommunity executes the bgp.community command sequence against the requested router.
func (s *LookingGlassService) BGPCommunity(ctx context.Context, req *connect.Request[pb.BGPCommunityRequest]) (*connect.Response[pb.BGPCommunityResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		logRPCError(rpcLogTag("BGPCommunity", rt, nil), "router_lookup", errs.UnknownRouter)
		return nil, errs.UnknownRouter
	}
	tag := rpcLogTag("BGPCommunity", rt, ri)
	start := time.Now()
	logRPCStart(tag)
	community := req.Msg.GetCommunity()
	if community == nil {
		logRPCError(tag, "community_nil", errs.OperationUnknown)
		return nil, errs.OperationUnknown
	}
	commStr, err := utils.SanitizeBGPCommunity(community.GetAsn(), community.GetValue())
	if err != nil {
		logRPCError(tag, "sanitize_community", err)
		return nil, err
	}
	key := bgpCommunityCacheKey(rt, commStr)
	var cached pb.BGPCommunityResponse
	if rpcCacheGet(ctx, key, &cached) {
		out := connect.NewResponse(&cached)
		out.Header().Set("X-Cache", "HIT")
		logRPCOK(tag, start, cached.GetResult(), parse.Result{Kind: cached.GetParserKind(), Status: cached.GetParseStatus()})
		return out, nil
	}

	// Try BMP first
	if bmp.IsBMPActive(ctx, rpcCacheClient, ri.Config.Name) {
		routes, err := bmp.FindBMPRoutesByCommunity(ctx, rpcCacheClient, ri.Config.Name, commStr)
		if err != nil {
			logRPCError(tag, "bgp_community_bmp_get", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("BMP datastore lookup failed"))
		}
		var pbPaths []*pb.BGPPath
		for _, r := range routes {
			pbPaths = append(pbPaths, &pb.BGPPath{
				Prefix:           r.Prefix,
				AsPath:           r.ASPath,
				Nexthop:          r.NextHop,
				Communities:      r.Communities,
				LargeCommunities: r.LargeCommunities,
			})
		}
		resp := &pb.BGPCommunityResponse{
			Result:    []byte("Served from stateless BMP Redis store\n"),
			Timestamp: nowPB(),
			Parsed: &pb.BGPPaths{
				Paths: pbPaths,
			},
			ParserKind:  pb.ParserKind_PARSER_KIND_BUILTIN,
			ParseStatus: pb.ParseStatus_PARSE_STATUS_OK,
		}
		rpcCacheSet(ctx, key, resp)
		logRPCOK(tag, start, resp.Result, parse.Result{Kind: pb.ParserKind_PARSER_KIND_BUILTIN, Status: pb.ParseStatus_PARSE_STATUS_OK})
		return connect.NewResponse(resp), nil
	}

	ret, err := ri.BGPCommunity(commStr)
	if err != nil {
		logRPCError(tag, "bgp_community_exec", err)
		if errors.Is(err, errs.PoolExhausted) {
			return nil, connect.NewError(connect.CodeResourceExhausted, err)
		}
		return nil, errs.ExecFailed
	}
	raw := joinSSH(ret)
	pr := runParser(ri, parse.OpBGPCommunity, raw)
	resp := &pb.BGPCommunityResponse{
		Result:      raw,
		Timestamp:   nowPB(),
		ParserKind:  pr.Kind,
		ParseStatus: pr.Status,
	}
	if bp, ok := pr.Payload.(*pb.BGPPaths); ok {
		resp.Parsed = bp
	}
	rpcCacheSet(ctx, key, resp)
	logRPCOK(tag, start, raw, pr)
	return connect.NewResponse(resp), nil
}

// BGPLargeCommunity executes the bgp.largecommunity command sequence against the requested router.
func (s *LookingGlassService) BGPLargeCommunity(ctx context.Context, req *connect.Request[pb.BGPLargeCommunityRequest]) (*connect.Response[pb.BGPLargeCommunityResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		logRPCError(rpcLogTag("BGPLargeCommunity", rt, nil), "router_lookup", errs.UnknownRouter)
		return nil, errs.UnknownRouter
	}
	tag := rpcLogTag("BGPLargeCommunity", rt, ri)
	start := time.Now()
	logRPCStart(tag)
	community := req.Msg.GetCommunity()
	if community == nil {
		logRPCError(tag, "community_nil", errs.OperationUnknown)
		return nil, errs.OperationUnknown
	}
	lc := utils.SanitizeBGPLargeCommunity(
		community.GetGlobalAdmin(),
		community.GetLocalData1(),
		community.GetLocalData2())
	key := bgpLargeCommunityCacheKey(rt, lc)
	var cached pb.BGPLargeCommunityResponse
	if rpcCacheGet(ctx, key, &cached) {
		out := connect.NewResponse(&cached)
		out.Header().Set("X-Cache", "HIT")
		logRPCOK(tag, start, cached.GetResult(), parse.Result{Kind: cached.GetParserKind(), Status: cached.GetParseStatus()})
		return out, nil
	}

	// Try BMP first
	if bmp.IsBMPActive(ctx, rpcCacheClient, ri.Config.Name) {
		routes, err := bmp.FindBMPRoutesByLargeCommunity(ctx, rpcCacheClient, ri.Config.Name, lc)
		if err != nil {
			logRPCError(tag, "bgp_largecommunity_bmp_get", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("BMP datastore lookup failed"))
		}
		var pbPaths []*pb.BGPPath
		for _, r := range routes {
			pbPaths = append(pbPaths, &pb.BGPPath{
				Prefix:           r.Prefix,
				AsPath:           r.ASPath,
				Nexthop:          r.NextHop,
				Communities:      r.Communities,
				LargeCommunities: r.LargeCommunities,
			})
		}
		resp := &pb.BGPLargeCommunityResponse{
			Result:    []byte("Served from stateless BMP Redis store\n"),
			Timestamp: nowPB(),
			Parsed: &pb.BGPPaths{
				Paths: pbPaths,
			},
			ParserKind:  pb.ParserKind_PARSER_KIND_BUILTIN,
			ParseStatus: pb.ParseStatus_PARSE_STATUS_OK,
		}
		rpcCacheSet(ctx, key, resp)
		logRPCOK(tag, start, resp.Result, parse.Result{Kind: pb.ParserKind_PARSER_KIND_BUILTIN, Status: pb.ParseStatus_PARSE_STATUS_OK})
		return connect.NewResponse(resp), nil
	}

	ret, err := ri.BGPLargeCommunity(lc)
	if err != nil {
		logRPCError(tag, "bgp_largecommunity_exec", err)
		if errors.Is(err, errs.PoolExhausted) {
			return nil, connect.NewError(connect.CodeResourceExhausted, err)
		}
		return nil, errs.ExecFailed
	}
	raw := joinSSH(ret)
	pr := runParser(ri, parse.OpBGPLargeCommunity, raw)
	resp := &pb.BGPLargeCommunityResponse{
		Result:      raw,
		Timestamp:   nowPB(),
		ParserKind:  pr.Kind,
		ParseStatus: pr.Status,
	}
	if bp, ok := pr.Payload.(*pb.BGPPaths); ok {
		resp.Parsed = bp
	}
	rpcCacheSet(ctx, key, resp)
	logRPCOK(tag, start, raw, pr)
	return connect.NewResponse(resp), nil
}

// BGPASPath executes the bgp.aspath command sequence against the requested router.
func (s *LookingGlassService) BGPASPath(ctx context.Context, req *connect.Request[pb.BGPASPathRequest]) (*connect.Response[pb.BGPASPathResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		logRPCError(rpcLogTag("BGPASPath", rt, nil), "router_lookup", errs.UnknownRouter)
		return nil, errs.UnknownRouter
	}
	tag := rpcLogTag("BGPASPath", rt, ri)
	start := time.Now()
	logRPCStart(tag)
	aspath, err := utils.SanitizeASPathRegex(req.Msg.GetPattern())
	if err != nil {
		logRPCError(tag, "sanitize_aspath", err)
		return nil, err
	}
	key := bgpASPathCacheKey(rt, aspath)
	var cached pb.BGPASPathResponse
	if rpcCacheGet(ctx, key, &cached) {
		out := connect.NewResponse(&cached)
		out.Header().Set("X-Cache", "HIT")
		logRPCOK(tag, start, cached.GetResult(), parse.Result{Kind: cached.GetParserKind(), Status: cached.GetParseStatus()})
		return out, nil
	}

	// Try BMP first
	if bmp.IsBMPActive(ctx, rpcCacheClient, ri.Config.Name) {
		routes, err := bmp.FindBMPRoutesByASPath(ctx, rpcCacheClient, ri.Config.Name, aspath)
		if err != nil {
			logRPCError(tag, "bgp_aspath_bmp_get", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("BMP datastore lookup failed"))
		}
		var pbPaths []*pb.BGPPath
		for _, r := range routes {
			pbPaths = append(pbPaths, &pb.BGPPath{
				Prefix:           r.Prefix,
				AsPath:           r.ASPath,
				Nexthop:          r.NextHop,
				Communities:      r.Communities,
				LargeCommunities: r.LargeCommunities,
			})
		}
		resp := &pb.BGPASPathResponse{
			Result:    []byte("Served from stateless BMP Redis store\n"),
			Timestamp: nowPB(),
			Parsed: &pb.BGPPaths{
				Paths: pbPaths,
			},
			ParserKind:  pb.ParserKind_PARSER_KIND_BUILTIN,
			ParseStatus: pb.ParseStatus_PARSE_STATUS_OK,
		}
		rpcCacheSet(ctx, key, resp)
		logRPCOK(tag, start, resp.Result, parse.Result{Kind: pb.ParserKind_PARSER_KIND_BUILTIN, Status: pb.ParseStatus_PARSE_STATUS_OK})
		return connect.NewResponse(resp), nil
	}

	ret, err := ri.BGPASPath(aspath)
	if err != nil {
		logRPCError(tag, "bgp_aspath_exec", err)
		if errors.Is(err, errs.PoolExhausted) {
			return nil, connect.NewError(connect.CodeResourceExhausted, err)
		}
		return nil, errs.ExecFailed
	}
	raw := joinSSH(ret)
	pr := runParser(ri, parse.OpBGPASPath, raw)
	resp := &pb.BGPASPathResponse{
		Result:      raw,
		Timestamp:   nowPB(),
		ParserKind:  pr.Kind,
		ParseStatus: pr.Status,
	}
	if bp, ok := pr.Payload.(*pb.BGPPaths); ok {
		resp.Parsed = bp
	}
	rpcCacheSet(ctx, key, resp)
	logRPCOK(tag, start, raw, pr)
	return connect.NewResponse(resp), nil
}
