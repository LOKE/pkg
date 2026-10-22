package lokerpc_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	v1 "github.com/LOKE/pkg/lokerpc"
	"github.com/LOKE/pkg/lokerpc/v2/internal/testpb"
	"github.com/LOKE/pkg/lokerpc/v2/internal/testpb/testpbconnect"
	"github.com/LOKE/pkg/lokerpc/v2/rpcv1"
	"github.com/go-kit/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
)

func TestDescriptorNegotiation(t *testing.T) {
	for _, paired := range []bool{false, true} {
		t.Run(fmt.Sprintf("paired=%t", paired), func(t *testing.T) {
			service := fixtureService("fixture", fixtureHandler{})
			service.NoBridge = paired
			legacy := []*v1.Service{v1.NewService("legacy", "legacy service", nil)}
			if paired {
				legacy = append(legacy, v1.NewService("fixture", "original help", nil))
			}
			mux := mountFixture(t, service, legacy...)
			baseline := http.NewServeMux()
			legacy[0].MountHandlers(log.NewNopLogger(), baseline)
			for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodHead} {
				got := serve(t, mux, method, "/rpc/legacy", "")
				want := serve(t, baseline, method, "/rpc/legacy", "")
				if got.Code != want.Code || !bytes.Equal(got.Body.Bytes(), want.Body.Bytes()) {
					t.Fatalf("legacy-only %s changed: %d %q, want %d %q", method, got.Code, got.Body.Bytes(), want.Code, want.Body.Bytes())
				}
				got.Header().Del("Vary")
				if !reflect.DeepEqual(got.Header(), want.Header()) {
					t.Fatalf("legacy-only %s headers changed: %v, want %v", method, got.Header(), want.Header())
				}
			}
			for _, test := range []struct {
				accept    string
				wantProto bool
			}{
				{"", false},
				{"*/*", false},
				{"application/*", false},
				{"application/json", false},
				{"application/proto", true},
				{"Application/Proto", true},
				{"application/proto;q=0", false},
				{"application/proto;q=0.1, application/json;q=1", true},
				{"application/json;q=0.9, application/proto", true},
				{"application/proto;q=0, application/proto;q=0.5", true},
				{" application/proto ; charset=binary ; q=0.5 ", true},
				{"application/proto;q=bad", false},
				{"application/proto;q=-1", false},
				{"application/proto;q=NaN", false},
				{"application/proto;q=2", false},
			} {
				t.Run(test.accept, func(t *testing.T) {
					for _, path := range []string{"/rpc/fixture", "/rpc/legacy", "/rpc"} {
						request := httptest.NewRequest(http.MethodGet, path, nil)
						request.Header.Set("Accept", test.accept)
						response := httptest.NewRecorder()
						mux.ServeHTTP(response, request)
						if response.Code != http.StatusOK {
							t.Fatalf("%s status = %d", path, response.Code)
						}
						if path != "/rpc" && response.Header().Get("Vary") != "Accept" {
							t.Errorf("%s Vary = %q", path, response.Header().Get("Vary"))
						}
						if test.wantProto && path == "/rpc/fixture" {
							if got := response.Header().Get("Content-Type"); got != "application/proto" {
								t.Fatalf("Content-Type = %q", got)
							}
							var descriptor rpcv1.ServiceDescriptor
							if err := proto.Unmarshal(response.Body.Bytes(), &descriptor); err != nil {
								t.Fatal(err)
							}
							files, err := protodesc.NewFiles(descriptor.GetFiles())
							if err != nil {
								t.Fatal(err)
							}
							if descriptor.GetService() != testpbconnect.FixtureServiceName {
								t.Fatalf("service = %q", descriptor.GetService())
							}
							if _, err := files.FindDescriptorByName(testpb.File_lokerpc_v2_internal_testpb_fixture_proto.Services().Get(0).FullName()); err != nil {
								t.Fatal(err)
							}
							seen := make(map[string]bool)
							for _, file := range descriptor.GetFiles().GetFile() {
								if seen[file.GetName()] {
									t.Fatalf("duplicate descriptor %q", file.GetName())
								}
								for _, dependency := range file.GetDependency() {
									if !seen[dependency] {
										t.Fatalf("dependency %q does not precede %q", dependency, file.GetName())
									}
								}
								seen[file.GetName()] = true
							}
						} else {
							if got := response.Header().Get("Content-Type"); got != v1.ContentType {
								t.Fatalf("%s Content-Type = %q", path, got)
							}
							if !json.Valid(response.Body.Bytes()) || bytes.Contains(response.Body.Bytes(), []byte(`"proto"`)) || bytes.Contains(response.Body.Bytes(), []byte(`"protoService"`)) {
								t.Fatalf("unexpected JSON metadata: %s", response.Body.Bytes())
							}
						}
					}
				})
			}
			for _, path := range []string{"/rpc/fixture", "/rpc/legacy"} {
				response := serve(t, mux, http.MethodPost, path, "")
				if response.Code != http.StatusMethodNotAllowed {
					t.Errorf("POST %s status = %d", path, response.Code)
				}
				if response.Header().Get("Vary") != "Accept" {
					t.Errorf("POST %s lacks Vary: Accept", path)
				}
			}
			request := httptest.NewRequest(http.MethodGet, "/rpc/fixture", nil)
			request.Header.Add("Accept", "application/json")
			request.Header.Add("Accept", "application/proto;q=0.5")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Header().Get("Content-Type") != "application/proto" {
				t.Error("multiple Accept headers did not negotiate proto")
			}
		})
	}
}
