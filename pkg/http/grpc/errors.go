package grpc

import (
	"context"
	"errors"

	"connectrpc.com/connect/v2"
	"github.com/AS203038/looking-glass/pkg/errs"
)

// publicErrors gives the code of each errs sentinel. Connect sends an error that is not a *connect.Error as "unknown" with no message.
var publicErrors = []struct {
	err  error
	code connect.Code
}{
	{errs.IPInvalid, connect.CodeInvalidArgument},
	{errs.NetInvalid, connect.CodeInvalidArgument},
	{errs.FamilyInvalid, connect.CodeInvalidArgument},
	{errs.ASPathMalformed, connect.CodeInvalidArgument},
	{errs.ASPathEmpty, connect.CodeInvalidArgument},
	{errs.ASPathTooLong, connect.CodeInvalidArgument},
	{errs.CommunityMalformed, connect.CodeInvalidArgument},
	{errs.PeerNameMalformed, connect.CodeInvalidArgument},
	{errs.UnknownRouter, connect.CodeNotFound},
	{errs.RouterUnavailable, connect.CodeUnavailable},
	{errs.OperationUnknown, connect.CodeUnimplemented},
	{errs.AuthFailed, connect.CodeUnavailable},
	{errs.ConnectionFailed, connect.CodeUnavailable},
	{errs.ExecFailed, connect.CodeUnavailable},
	{errs.ExecTimeout, connect.CodeDeadlineExceeded},
	{errs.ExecCanceled, connect.CodeCanceled},
	{errs.PoolExhausted, connect.CodeResourceExhausted},
}

// publicErrorInterceptor sends each errs sentinel with its code and its text. Other errors stay "unknown" with no message.
func publicErrorInterceptor(next connect.ServerFunc) connect.ServerFunc {
	return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
		err := next(ctx, spec, stream)
		var ce *connect.Error
		if err == nil || errors.As(err, &ce) {
			return err
		}
		for _, p := range publicErrors {
			if errors.Is(err, p.err) {
				return connect.NewError(p.code, p.err.Error()).WithCause(err)
			}
		}
		return err
	}
}

// execError keeps the router errors that tell the client what to do and returns errs.ExecFailed for the others.
func execError(err error) error {
	if errors.Is(err, errs.PoolExhausted) || errors.Is(err, errs.ExecTimeout) || errors.Is(err, errs.ExecCanceled) {
		return err
	}
	return errs.ExecFailed
}
