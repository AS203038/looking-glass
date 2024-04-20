package grpc

import (
	"context"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	pb "gitlab.as203038.net/AS203038/looking-glass/protobuf/lookingglass/v0"
	"gitlab.as203038.net/AS203038/looking-glass/protobuf/lookingglass/v0/lookingglassconnect"
	"gitlab.as203038.net/AS203038/looking-glass/server/errs"
	"gitlab.as203038.net/AS203038/looking-glass/server/utils"
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
	return connect.NewResponse(&pb.PingResponse{
		Result: strings.Join(ret, "\n"),
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
	return connect.NewResponse(&pb.TracerouteResponse{
		Result: strings.Join(ret, "\n"),
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
	return connect.NewResponse(&pb.BGPRouteResponse{
		Result: strings.Join(ret, "\n"),
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
	return connect.NewResponse(&pb.BGPCommunityResponse{
		Result: strings.Join(ret, "\n"),
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
	return connect.NewResponse(&pb.BGPASPathResponse{
		Result: strings.Join(ret, "\n"),
	}), nil
}
