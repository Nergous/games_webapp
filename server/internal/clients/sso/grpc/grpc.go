package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	ssov1 "github.com/Nergous/sso_protos/gen/go/sso"

	grpclog "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpcretry "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	api ssov1.AuthClient
	log *slog.Logger
}

func New(
	ctx context.Context,
	log *slog.Logger,
	addr string,
	timeout time.Duration,
	retriesCount int,
) (*Client, error) {
	const op = "grpc.New"

	retryOpts := []grpcretry.CallOption{
		grpcretry.WithCodes(codes.NotFound, codes.Aborted, codes.DeadlineExceeded),
		grpcretry.WithMax(uint(retriesCount)),
		grpcretry.WithPerRetryTimeout(timeout),
	}

	logOpts := []grpclog.Option{
		grpclog.WithLogOnEvents(grpclog.PayloadReceived, grpclog.PayloadSent),
	}

	cc, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			grpclog.UnaryClientInterceptor(InterceptorLogger(log), logOpts...),
			grpcretry.UnaryClientInterceptor(retryOpts...),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &Client{
		api: ssov1.NewAuthClient(cc),
		log: log,
	}, nil
}

func InterceptorLogger(l *slog.Logger) grpclog.Logger {
	return grpclog.LoggerFunc(func(ctx context.Context, lvl grpclog.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(lvl), msg, fields...)
	})
}

func (c *Client) ValidateToken(ctx context.Context, token string) (int64, bool, error) {
	resp, err := c.api.ValidateToken(ctx, &ssov1.ValidateTokenRequest{Token: token})
	if err != nil {
		c.log.Error("sso.ValidateToken failed", slog.String("error", err.Error()))
		return 0, false, err
	}

	return resp.GetUserId(), resp.GetValid(), nil
}

func (c *Client) Register(ctx context.Context, email, password, steamURL, pathToPhoto string) (int64, error) {
	resp, err := c.api.Register(ctx, &ssov1.RegisterRequest{Email: email, Password: password, SteamUrl: steamURL, PathToPhoto: pathToPhoto})
	if err != nil {
		c.log.Error("sso.Register failed", slog.String("error", err.Error()))
		return 0, err
	}

	return resp.GetUserId(), nil
}

func (c *Client) Login(ctx context.Context, email, password string, appID int32) (string, error) {
	resp, err := c.api.Login(ctx, &ssov1.LoginRequest{Email: email, Password: password, AppId: appID})
	if err != nil {
		c.log.Error("sso.Login failed", slog.String("error", err.Error()))
		return "", err
	}

	return resp.GetToken(), nil
}

func (c *Client) GetUserInfo(ctx context.Context, userID int64) (string, string, string, error) {
	resp, err := c.api.UserInfo(ctx, &ssov1.UserInfoRequest{UserId: userID})
	if err != nil {
		c.log.Error("sso.UserInfo failed", slog.String("error", err.Error()))
		return "", "", "", err
	}

	return resp.GetEmail(), resp.GetSteamUrl(), resp.GetPathToPhoto(), nil
}
