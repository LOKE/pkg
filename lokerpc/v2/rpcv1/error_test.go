package rpcv1_test

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/LOKE/pkg/lokerpc/v2/rpcv1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestFromError(t *testing.T) {
	want := &rpcv1.Error{Code: "product_not_found", Namespace: "menu", Type: "menu/product_not_found", Instance: "instance", Expose: true, Help: "choose another"}
	standard, err := connect.NewErrorDetail(want)
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := connect.NewErrorDetail(&emptypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	without := connect.NewError(connect.CodeUnknown, errors.New("without standard detail"))
	without.AddDetail(unrelated)
	with := connect.NewError(connect.CodeNotFound, errors.New("with standard detail"))
	with.AddDetail(unrelated)
	with.AddDetail(standard)
	for _, test := range []struct {
		name string
		err  error
		want *rpcv1.Error
	}{
		{name: "nil"},
		{name: "ordinary", err: errors.New("failure")},
		{name: "Connect without details", err: connect.NewError(connect.CodeUnknown, errors.New("failure"))},
		{name: "unrelated detail", err: without},
		{name: "standard after unrelated detail", err: with, want: want},
		{name: "wrapped", err: fmt.Errorf("outer: %w", with), want: want},
		{name: "nested Connect", err: connect.NewError(connect.CodeAborted, with), want: want},
		{name: "joined", err: errors.Join(errors.New("other"), with), want: want},
		{name: "joined after unrelated Connect", err: errors.Join(without, with), want: want},
		{name: "wrapped joined", err: fmt.Errorf("outer: %w", errors.Join(without, with)), want: want},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := rpcv1.FromError(test.err)
			if ok != (test.want != nil) || !proto.Equal(got, test.want) {
				t.Errorf("FromError() = %v, %v; want %v, %v", got, ok, test.want, test.want != nil)
			}
		})
	}
}

func TestDependenciesExcludeServerPackages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "list", "-deps", "github.com/LOKE/pkg/lokerpc/v2/rpcv1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps rpcv1: %v\n%s", err, output)
	}
	for _, dependency := range strings.Fields(string(output)) {
		if dependency == "github.com/LOKE/pkg/lokerpc" || dependency == "github.com/LOKE/pkg/lokerpc/v2" {
			t.Errorf("rpcv1 imports server package %q", dependency)
		}
	}
}
