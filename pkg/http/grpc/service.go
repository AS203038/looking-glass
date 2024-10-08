package grpc

import (
	"context"
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
		return nil, errs.UnknownRouter
	}
	target, err := utils.NewIPNetFromProtobuf(req.Msg.GetTarget())
	if err != nil {
		return nil, err
	}
	ret, err := ri.Ping(target)
	if err != nil {
		return nil, err
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
		return nil, errs.UnknownRouter
	}
	target, err := utils.NewIPNetFromProtobuf(req.Msg.GetTarget())
	if err != nil {
		return nil, err
	}
	ret, err := ri.Traceroute(target)
	if err != nil {
		return nil, err
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
		return nil, errs.UnknownRouter
	}
	target, err := utils.NewIPNetFromProtobuf(req.Msg.GetTarget())
	if err != nil {
		return nil, err
	}
	ret, err := ri.BGPRoute(target)
	if err != nil {
		return nil, err
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
		return nil, errs.UnknownRouter
	}
	community := req.Msg.GetCommunity()
	ret, err := ri.BGPCommunity(strconv.Itoa(int(community.Asn)) + ":" + strconv.Itoa(int(community.Value)))
	if err != nil {
		return nil, err
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

func (s *LookingGlassService) BGPASPath(ctx context.Context, req *connect.Request[pb.BGPASPathRequest]) (*connect.Response[pb.BGPASPathResponse], error) {
	rt := req.Msg.GetRouterId()
	ri, ok := s.rts.GetByID(rt)
	if !ok {
		return nil, errs.UnknownRouter
	}
	aspath, err := utils.SanitizeASPathRegex(req.Msg.GetPattern())
	if err != nil {
		return nil, err
	}
	ret, err := ri.BGPASPath(aspath)
	if err != nil {
		return nil, err
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
