// Command negotiate is a toy app showing lokerpc servers and clients
// negotiating a binary wire format when both sides support it.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/LOKE/pkg/examples/negotiate/protocodec"
	"github.com/LOKE/pkg/lokerpc"
	kitlog "github.com/go-kit/log"
)

func mountServer(service *lokerpc.Service) *httptest.Server {
	mux := http.NewServeMux()
	lokerpc.MountHandlers(kitlog.NewNopLogger(), mux, service)
	return httptest.NewServer(mux)
}

func main() {
	proto := protocodec.New()

	newCodecServer := mountServer(newMenuService(proto))
	defer newCodecServer.Close()
	oldServer := mountServer(newMenuService())
	defer oldServer.Close()

	scenarios := []struct {
		name   string
		server *httptest.Server
		opts   []lokerpc.ClientOption
	}{
		{"plain JSON client -> codec server", newCodecServer, nil},
		{"protobuf client -> codec server", newCodecServer, []lokerpc.ClientOption{lokerpc.ClientCodecs(proto)}},
		{"protobuf client -> old server", oldServer, []lokerpc.ClientOption{lokerpc.ClientCodecs(proto)}},
	}

	for _, scenario := range scenarios {
		recorder := &Recorder{}
		opts := append(scenario.opts, lokerpc.WithHTTPClient(&http.Client{Transport: recorder}))
		client := MenuClient{lokerpc.NewClient(scenario.server.URL+"/rpc/menu", opts...)}

		fmt.Printf("== %s\n", scenario.name)
		runScenario(client, recorder)
	}
}

func runScenario(client MenuClient, recorder *Recorder) {
	ctx := context.Background()
	menu := seedMenu(3)

	saved, err := client.SaveMenu(ctx, menu)
	if err != nil {
		log.Fatal(err)
	}
	logExchanges("saveMenu", recorder)
	fmt.Printf("   saved revision %d at %s\n", saved.Revision, saved.SavedAt.Format("15:04:05.000"))

	got, err := client.GetMenu(ctx, GetMenuRequest{MenuID: menu.ID})
	if err != nil {
		log.Fatal(err)
	}
	logExchanges("getMenu", recorder)
	fmt.Printf("   fetched %q with %d sections\n", got.Name, len(got.Sections))

	_, err = client.GetMenu(ctx, GetMenuRequest{MenuID: "missing"})
	logExchanges("getMenu (missing)", recorder)
	fmt.Printf("   error: %v\n", err)
}

func logExchanges(call string, recorder *Recorder) {
	for _, exchange := range recorder.Take() {
		fmt.Printf("   %-18s %-7s %-22s status=%d req=%q resp=%q resp-bytes=%d %s\n",
			call, exchange.Method, exchange.Path, exchange.Status, exchange.RequestType,
			exchange.ResponseType, len(exchange.ResponseBody), preview(exchange.ResponseBody))
	}
}

func preview(body []byte) string {
	const previewLen = 24
	if len(body) > previewLen {
		return fmt.Sprintf("%x...", body[:previewLen])
	}
	return fmt.Sprintf("%x", body)
}
