package grpc

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/AS203038/looking-glass/pkg/errs"
	"github.com/AS203038/looking-glass/pkg/utils"
	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"github.com/AS203038/looking-glass/protobuf/lookingglass/v0/lookingglassconnect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

// rpcLogTag builds a compact server-side log tag for a handler failure.
// It intentionally excludes any client-supplied free-form values that
// could be sensitive; only structured identifiers are surfaced.
func rpcLogTag(op string, routerID int64, ri *utils.RouterInstance) string {
	tag := "op=" + op + " router_id=" + strconv.FormatInt(routerID, 10)
	if ri != nil && ri.Config != nil {
		tag += " router=" + ri.Config.Name + " type=" + ri.Config.Type
	}
	return tag
}

// logRPCError writes a detailed server-side line describing a failed RPC
// invocation. The underlying Go error (which may carry extra context
// written by SSHExec / template rendering) is included verbatim. The
// caller still returns a generic sentinel error to the client.
func logRPCError(tag, stage string, err error) {
	log.Printf("RPC: %s stage=%s: %v", tag, stage, err)
}

type LookingGlassService struct {
	lookingglassconnect.UnimplementedLookingGlassServiceHandler
	ctx context.Context
	rts utils.RouterMap
}

func NewLookingGlassService(ctx context.Context, rts utils.RouterMap) lookingglassconnect.LookingGlassServiceHandler {
	return &LookingGlassService{
		ctx: ctx,
		rts: rts,
	}
}

func (s *LookingGlassService) GetInfo(ctx context.Context, req *connect.Request[emptypb.Empty]) (*connect.Response[pb.GetInfoResponse], error) {
	h, err := os.Hostname()
	if err != nil {
		h = "unknown"
	}
	return connect.NewResponse(&pb.GetInfoResponse{
		Hostname: h,
		Version:  utils.Version(),
	}), nil
}

func (s *LookingGlassService) GetRouters(ctx context.Context, req *connect.Request[pb.GetRoutersRequest]) (*connect.Response[pb.GetRoutersResponse], error) {
	var ret []*pb.Router
	lim := req.Msg.GetLimit()
	page := req.Msg.GetPageToken()
	len := uint32(len(s.rts))
	if lim == 0 {
		lim = 10
	}
	if page == 0 {
		page = 1
	}
	start := (page - 1) * lim
	end := start + lim
	if end > len {
		end = len
	}
	if start > len {
		return connect.NewResponse(&pb.GetRoutersResponse{}), nil
	}
	for k, v := range s.rts[start:end] {
		ret = append(ret, &pb.Router{
			Name:     v.Config.Name,
			Location: v.Config.Location,
			Id:       int64(k + 1),
			Health: &pb.RouterHealth{
				Healthy: v.HealthCheck.Healthy,
				Timestamp: &timestamppb.Timestamp{
					Seconds: v.HealthCheck.Checked.Unix(),
					Nanos:   int32(v.HealthCheck.Checked.Nanosecond()),
				},
			},
		})
	}
	var nextPage uint32
	if end < len {
		np := page + 1
		nextPage = np
	}
	return connect.NewResponse(&pb.GetRoutersResponse{
		Routers:  ret,
		NextPage: nextPage,
	}), nil
}

func (s *LookingGlassService) Ping(ctx context.Context, req *connect.Request[pb.PingRequest]) (*connect.Response[pb.PingResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		logRPCError(rpcLogTag("Ping", rt, nil), "router_lookup", errs.UnknownRouter)
		return nil, errs.UnknownRouter
	}
	tag := rpcLogTag("Ping", rt, ri)
	target, err := utils.NewIPNetFromProtobuf(req.Msg.GetTarget())
	if err != nil {
		logRPCError(tag, "parse_target", err)
		return nil, errs.IPInvalid
	}
	ret, err := ri.Ping(target)
	if err != nil {
		logRPCError(tag, "ping_exec", err)
		return nil, errs.ExecFailed
	}
	ts := time.Now()
	return connect.NewResponse(&pb.PingResponse{
		Result: []byte(strings.Join(ret, "\n")),
		Timestamp: &timestamppb.Timestamp{
			Seconds: ts.Unix(),
			Nanos:   int32(ts.Nanosecond()),
		},
	}), nil
}

func (s *LookingGlassService) Traceroute(ctx context.Context, req *connect.Request[pb.TracerouteRequest]) (*connect.Response[pb.TracerouteResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		logRPCError(rpcLogTag("Traceroute", rt, nil), "router_lookup", errs.UnknownRouter)
		return nil, errs.UnknownRouter
	}
	tag := rpcLogTag("Traceroute", rt, ri)
	target, err := utils.NewIPNetFromProtobuf(req.Msg.GetTarget())
	if err != nil {
		logRPCError(tag, "parse_target", err)
		return nil, errs.IPInvalid
	}
	ret, err := ri.Traceroute(target)
	if err != nil {
		logRPCError(tag, "traceroute_exec", err)
		return nil, errs.ExecFailed
	}
	ts := time.Now()
	return connect.NewResponse(&pb.TracerouteResponse{
		Result: []byte(strings.Join(ret, "\n")),
		Timestamp: &timestamppb.Timestamp{
			Seconds: ts.Unix(),
			Nanos:   int32(ts.Nanosecond()),
		},
	}), nil
}

func (s *LookingGlassService) BGPRoute(ctx context.Context, req *connect.Request[pb.BGPRouteRequest]) (*connect.Response[pb.BGPRouteResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		logRPCError(rpcLogTag("BGPRoute", rt, nil), "router_lookup", errs.UnknownRouter)
		return nil, errs.UnknownRouter
	}
	tag := rpcLogTag("BGPRoute", rt, ri)
	target, err := utils.NewIPNetFromProtobuf(req.Msg.GetTarget())
	if err != nil {
		logRPCError(tag, "parse_target", err)
		return nil, errs.IPInvalid
	}
	ret, err := ri.BGPRoute(target)
	if err != nil {
		logRPCError(tag, "bgp_route_exec", err)
		return nil, errs.ExecFailed
	}
	ts := time.Now()
	return connect.NewResponse(&pb.BGPRouteResponse{
		Result: []byte(strings.Join(ret, "\n")),
		Timestamp: &timestamppb.Timestamp{
			Seconds: ts.Unix(),
			Nanos:   int32(ts.Nanosecond()),
		},
	}), nil
}

func (s *LookingGlassService) BGPCommunity(ctx context.Context, req *connect.Request[pb.BGPCommunityRequest]) (*connect.Response[pb.BGPCommunityResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		logRPCError(rpcLogTag("BGPCommunity", rt, nil), "router_lookup", errs.UnknownRouter)
		return nil, errs.UnknownRouter
	}
	tag := rpcLogTag("BGPCommunity", rt, ri)
	community := req.Msg.GetCommunity()
	if community == nil {
		logRPCError(tag, "community_nil", errs.OperationUnknown)
		return nil, errs.OperationUnknown
	}
	ret, err := ri.BGPCommunity(strconv.Itoa(int(community.Asn)) + ":" + strconv.Itoa(int(community.Value)))
	if err != nil {
		logRPCError(tag, "bgp_community_exec", err)
		return nil, errs.ExecFailed
	}
	ts := time.Now()
	return connect.NewResponse(&pb.BGPCommunityResponse{
		Result: []byte(strings.Join(ret, "\n")),
		Timestamp: &timestamppb.Timestamp{
			Seconds: ts.Unix(),
			Nanos:   int32(ts.Nanosecond()),
		},
	}), nil
}

func (s *LookingGlassService) BGPLargeCommunity(ctx context.Context, req *connect.Request[pb.BGPLargeCommunityRequest]) (*connect.Response[pb.BGPLargeCommunityResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		logRPCError(rpcLogTag("BGPLargeCommunity", rt, nil), "router_lookup", errs.UnknownRouter)
		return nil, errs.UnknownRouter
	}
	tag := rpcLogTag("BGPLargeCommunity", rt, ri)
	community := req.Msg.GetCommunity()
	if community == nil {
		logRPCError(tag, "community_nil", errs.OperationUnknown)
		return nil, errs.OperationUnknown
	}
	lc := strconv.FormatUint(uint64(community.GetGlobalAdmin()), 10) + ":" +
		strconv.FormatUint(uint64(community.GetLocalData1()), 10) + ":" +
		strconv.FormatUint(uint64(community.GetLocalData2()), 10)
	ret, err := ri.BGPLargeCommunity(lc)
	if err != nil {
		logRPCError(tag, "bgp_largecommunity_exec", err)
		return nil, errs.ExecFailed
	}
	ts := time.Now()
	return connect.NewResponse(&pb.BGPLargeCommunityResponse{
		Result: []byte(strings.Join(ret, "\n")),
		Timestamp: &timestamppb.Timestamp{
			Seconds: ts.Unix(),
			Nanos:   int32(ts.Nanosecond()),
		},
	}), nil
}

func (s *LookingGlassService) BGPASPath(ctx context.Context, req *connect.Request[pb.BGPASPathRequest]) (*connect.Response[pb.BGPASPathResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		logRPCError(rpcLogTag("BGPASPath", rt, nil), "router_lookup", errs.UnknownRouter)
		return nil, errs.UnknownRouter
	}
	tag := rpcLogTag("BGPASPath", rt, ri)
	aspath, err := utils.SanitizeASPathRegex(req.Msg.GetPattern())
	if err != nil {
		logRPCError(tag, "sanitize_aspath", err)
		return nil, err
	}
	ret, err := ri.BGPASPath(aspath)
	if err != nil {
		logRPCError(tag, "bgp_aspath_exec", err)
		return nil, errs.ExecFailed
	}
	ts := time.Now()
	return connect.NewResponse(&pb.BGPASPathResponse{
		Result: []byte(strings.Join(ret, "\n")),
		Timestamp: &timestamppb.Timestamp{
			Seconds: ts.Unix(),
			Nanos:   int32(ts.Nanosecond()),
		},
	}), nil
}
