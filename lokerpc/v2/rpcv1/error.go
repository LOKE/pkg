package rpcv1

import "connectrpc.com/connect"

// FromError returns the standard LOKE detail attached to a Connect error or its causes.
func FromError(err error) (*Error, bool) {
	for err != nil {
		if connectErr, ok := err.(*connect.Error); ok {
			for _, detail := range connectErr.Details() {
				if detail.Type() != "loke.rpc.v1.Error" {
					continue
				}
				value, valueErr := detail.Value()
				if valueErr != nil {
					continue
				}
				if result, ok := value.(*Error); ok {
					return result, true
				}
			}
		}
		switch wrapped := err.(type) {
		case interface{ Unwrap() []error }:
			for _, cause := range wrapped.Unwrap() {
				if detail, ok := FromError(cause); ok {
					return detail, true
				}
			}
			return nil, false
		case interface{ Unwrap() error }:
			err = wrapped.Unwrap()
		default:
			return nil, false
		}
	}
	return nil, false
}
