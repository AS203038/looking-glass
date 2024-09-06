package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	pb "github.com/AS203038/looking-glass/protobuf/lookingglass/v0"
	"github.com/AS203038/looking-glass/protobuf/lookingglass/v0/lookingglassconnect"
	yaml "gopkg.in/yaml.v2"
)

type LookingGlassIndex struct {
	LookingGlasses []LookingGlass `yaml:"index"`
}

type LookingGlass struct {
	ASN  string `yaml:"asn"`
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type LGRequest struct {
	RouterID  int64
	Operation string
	Params    string
	UseJSON   bool
}

type Return struct {
	Result    string `json:"result"`
	Timestamp string `json:"timestamp"`
}

type RouterReturn struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Location  string `json:"location"`
	Healthy   bool   `json:"healthy"`
	Timestamp string `json:"timestamp"`
}

var (
	LookingGlassIndexURL = "https://raw.githubusercontent.com/AS203038/looking-glass/main/public_index.yaml"
	lookingGlass         *LookingGlass
	lgParam              string
	lgRequest            = &LGRequest{RouterID: 1}
	ctx                  context.Context
	cancel               context.CancelFunc
)

func init() {
	flag.StringVar(&LookingGlassIndexURL, "index", LookingGlassIndexURL, "URL of the Looking Glass index")
	flag.StringVar(&lgParam, "lg", "", "Looking Glass name/url to query")
	flag.Int64Var(&lgRequest.RouterID, "router", lgRequest.RouterID, "Router ID")
	flag.StringVar(&lgRequest.Operation, "op", lgRequest.Operation, "Operation to perform: get_routers, ping, traceroute, bgp_route, bgp_community, bgp_aspath")
	flag.StringVar(&lgRequest.Params, "param", lgRequest.Params, "Operation parameter")
	flag.BoolVar(&lgRequest.UseJSON, "json", lgRequest.UseJSON, "Output in JSON format")
	flag.Parse()

	if flag.NArg() == 2 && (lgRequest.Operation == "" && lgRequest.Params == "") {
		lgRequest.Operation = flag.Arg(0)
		lgRequest.Params = flag.Arg(1)
	}

	if lgRequest.Operation == "" || lgParam == "" {
		printUsageAndExit()
	}

	var err error
	parsedURL, err := url.Parse(lgParam)
	if err != nil || parsedURL.Scheme == "" {
		lookingGlass, err = getLookingGlass(lgParam)
		if err != nil {
			fmt.Printf("Error getting Looking Glass: %s\n", err)
			os.Exit(1)
		}
	} else {
		lookingGlass = &LookingGlass{
			Name: lgParam,
			URL:  lgParam,
		}
	}

	ctx, cancel = context.WithCancel(context.Background())
	setupSignalHandler()
}

func main() {
	client := lookingglassconnect.NewLookingGlassServiceClient(http.DefaultClient, lookingGlass.URL)
	var ret string
	var ts time.Time
	var err error

	switch lgRequest.Operation {
	case "get_routers":
		err = handleGetRouters(client)
	case "ping":
		ret, ts, err = handlePing(client)
	case "traceroute":
		ret, ts, err = handleTraceroute(client)
	case "bgp_route":
		ret, ts, err = handleBGPRoute(client)
	case "bgp_community":
		ret, ts, err = handleBGPCommunity(client)
	case "bgp_aspath":
		ret, ts, err = handleBGPASPath(client)
	default:
		err = fmt.Errorf("unknown operation: %s", lgRequest.Operation)
	}

	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	printResult(ret, ts)
}

func printUsageAndExit() {
	fmt.Printf("No operation or Looking Glass specified\n")
	flag.Usage()
	os.Exit(1)
}

func setupSignalHandler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		cancel()
		time.Sleep(15 * time.Second)
		fmt.Println("Shutting down")
		os.Exit(2)
	}()
}

func getLookingGlassIndex() (*LookingGlassIndex, error) {
	resp, err := http.Get(LookingGlassIndexURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	var index LookingGlassIndex
	err = yaml.NewDecoder(resp.Body).Decode(&index)
	if err != nil {
		return nil, err
	}
	return &index, nil
}

func getLookingGlass(name string) (*LookingGlass, error) {
	index, err := getLookingGlassIndex()
	if err != nil {
		return nil, err
	}
	name = strings.ToLower(name)
	for _, lg := range index.LookingGlasses {
		lg.Name = strings.ToLower(lg.Name)
		if lg.Name == name || lg.ASN == name || lg.ASN == strings.TrimPrefix(name, "as") {
			return &lg, nil
		}
	}
	return nil, fmt.Errorf("looking glass not found: %s", name)
}

func handleGetRouters(client lookingglassconnect.LookingGlassServiceClient) error {
	rts, err := getRouters(client, 1)
	if err != nil {
		return err
	}
	for _, rt := range rts {
		if lgRequest.UseJSON {
			rtJSON, _ := json.Marshal(&RouterReturn{
				ID:        rt.GetId(),
				Name:      rt.GetName(),
				Location:  rt.GetLocation(),
				Healthy:   rt.Health.GetHealthy(),
				Timestamp: rt.Health.GetTimestamp().AsTime().Format(time.RFC3339),
			})
			fmt.Println(string(rtJSON))
		} else {
			fmt.Printf("%d: %s (%s): %v\n", rt.GetId(), rt.GetName(), rt.GetLocation(), rt.Health.GetHealthy())
		}
	}
	os.Exit(0) //Yeah this is ugly
	return nil
}

func handlePing(client lookingglassconnect.LookingGlassServiceClient) (string, time.Time, error) {
	ping, err := client.Ping(ctx, connect.NewRequest(&pb.PingRequest{
		RouterId: lgRequest.RouterID,
		Target:   lgRequest.Params,
	}))
	if err != nil {
		return "", time.Time{}, err
	}
	return string(ping.Msg.GetResult()), ping.Msg.Timestamp.AsTime(), nil
}

func handleTraceroute(client lookingglassconnect.LookingGlassServiceClient) (string, time.Time, error) {
	traceroute, err := client.Traceroute(ctx, connect.NewRequest(&pb.TracerouteRequest{
		RouterId: lgRequest.RouterID,
		Target:   lgRequest.Params,
	}))
	if err != nil {
		return "", time.Time{}, err
	}
	return string(traceroute.Msg.GetResult()), traceroute.Msg.Timestamp.AsTime(), nil
}

func handleBGPRoute(client lookingglassconnect.LookingGlassServiceClient) (string, time.Time, error) {
	bgpRoute, err := client.BGPRoute(ctx, connect.NewRequest(&pb.BGPRouteRequest{
		RouterId: lgRequest.RouterID,
		Target:   lgRequest.Params,
	}))
	if err != nil {
		return "", time.Time{}, err
	}
	return string(bgpRoute.Msg.GetResult()), bgpRoute.Msg.Timestamp.AsTime(), nil
}

func handleBGPCommunity(client lookingglassconnect.LookingGlassServiceClient) (string, time.Time, error) {
	params := strings.SplitN(lgRequest.Params, ":", 2)
	if len(params) != 2 {
		return "", time.Time{}, fmt.Errorf("invalid parameter: %s", lgRequest.Params)
	}
	asn, err := strconv.ParseInt(params[0], 10, 32)
	if err != nil {
		return "", time.Time{}, err
	}
	val, err := strconv.ParseInt(params[1], 10, 32)
	if err != nil {
		return "", time.Time{}, err
	}
	bgpCommunity, err := client.BGPCommunity(ctx, connect.NewRequest(&pb.BGPCommunityRequest{
		RouterId: lgRequest.RouterID,
		Community: &pb.BGPCommunity{
			Asn:   int32(asn),
			Value: int32(val),
		},
	}))
	if err != nil {
		return "", time.Time{}, err
	}
	return string(bgpCommunity.Msg.GetResult()), bgpCommunity.Msg.Timestamp.AsTime(), nil
}

func handleBGPASPath(client lookingglassconnect.LookingGlassServiceClient) (string, time.Time, error) {
	bgpASPath, err := client.BGPASPath(ctx, connect.NewRequest(&pb.BGPASPathRequest{
		RouterId: lgRequest.RouterID,
		Pattern:  lgRequest.Params,
	}))
	if err != nil {
		return "", time.Time{}, err
	}
	return string(bgpASPath.Msg.GetResult()), bgpASPath.Msg.Timestamp.AsTime(), nil
}

func getRouters(client lookingglassconnect.LookingGlassServiceClient, page uint32) ([]*pb.Router, error) {
	var ret []*pb.Router
	rts, err := client.GetRouters(ctx, connect.NewRequest(&pb.GetRoutersRequest{
		Limit:     1024,
		PageToken: page,
	}))
	if err != nil {
		return nil, err
	}
	ret = append(ret, rts.Msg.GetRouters()...)
	if rts.Msg.GetNextPage() != 0 {
		next, err := getRouters(client, rts.Msg.GetNextPage())
		if err != nil {
			return nil, err
		}
		ret = append(ret, next...)
	}
	return ret, nil
}

func printResult(ret string, ts time.Time) {
	if lgRequest.UseJSON {
		retJSON, _ := json.Marshal(&Return{
			Result:    ret,
			Timestamp: ts.Format(time.RFC3339),
		})
		fmt.Println(string(retJSON))
	} else {
		fmt.Print(ret)
		fmt.Println(ts.Format(time.RFC3339))
	}
}
