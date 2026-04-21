// Package grpc wraps the SSO gRPC client with project-wide retry/logging
// interceptors. Method wrappers here deliberately do NOT log errors — callers
// (middleware, controllers via WriteError) are the single source of truth for
// error logging, and duplicating here produces noise without added context.
package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"time"

	ssov1 "github.com/Nergous/sso_protos/gen/go/sso"

	grpclog "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpcretry "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	cc   *grpc.ClientConn
	auth ssov1.AuthClient
	app  ssov1.AppClient
	user ssov1.UserClient
	log  *slog.Logger
}

// New constructs a gRPC client with chained logging + retry interceptors.
// When insecureTransport is true the connection is plaintext — intended for
// local development only; production deployments must pass false so a real
// TLS transport is used.
func New(
	ctx context.Context,
	log *slog.Logger,
	addr string,
	timeout time.Duration,
	retriesCount int,
	insecureTransport bool,
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

	var transport grpc.DialOption
	if insecureTransport {
		transport = grpc.WithTransportCredentials(insecure.NewCredentials())
	} else {
		transport = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12}))
	}

	cc, err := grpc.NewClient(
		addr,
		transport,
		grpc.WithChainUnaryInterceptor(
			grpclog.UnaryClientInterceptor(InterceptorLogger(log), logOpts...),
			grpcretry.UnaryClientInterceptor(retryOpts...),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &Client{
		cc:   cc,
		auth: ssov1.NewAuthClient(cc),
		app:  ssov1.NewAppClient(cc),
		user: ssov1.NewUserClient(cc),
		log:  log,
	}, nil
}

// Close releases the underlying gRPC connection. Safe to call at shutdown.
func (c *Client) Close() error {
	if c.cc == nil {
		return nil
	}
	return c.cc.Close()
}

func InterceptorLogger(l *slog.Logger) grpclog.Logger {
	return grpclog.LoggerFunc(func(ctx context.Context, lvl grpclog.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(lvl), msg, fields...)
	})
}

func (c *Client) ValidateToken(ctx context.Context, token string) (uint32, bool, error) {
	resp, err := c.auth.ValidateToken(ctx, &ssov1.ValidateTokenRequest{Token: token})
	if err != nil {
		return 0, false, err
	}
	return resp.GetUserId(), resp.GetValid(), nil
}

func (c *Client) Register(ctx context.Context, email, password, steamURL, pathToPhoto string) (uint32, error) {
	resp, err := c.auth.Register(ctx, &ssov1.RegisterRequest{Email: email, Password: password, SteamUrl: steamURL, PathToPhoto: pathToPhoto})
	if err != nil {
		return 0, err
	}
	return resp.GetUserId(), nil
}

func (c *Client) Login(ctx context.Context, email, password string, appID uint32) (accessToken string, refreshToken string, err error) {
	resp, err := c.auth.Login(ctx, &ssov1.LoginRequest{Email: email, Password: password, AppId: appID})
	if err != nil {
		return "", "", err
	}
	return resp.GetAccessToken(), resp.GetRefreshToken(), nil
}

func (c *Client) Logout(ctx context.Context, refreshToken string) error {
	_, err := c.auth.Logout(ctx, &ssov1.LogoutRequest{Token: refreshToken})
	return err
}

func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (accessToken, newRefreshToken string, err error) {
	resp, err := c.auth.Refresh(ctx, &ssov1.RefreshRequest{
		RefreshToken: refreshToken,
	})
	if err != nil {
		return "", "", err
	}
	return resp.GetAccessToken(), resp.GetRefreshToken(), nil
}

func (c *Client) IsAdmin(ctx context.Context, userID uint32, appID uint32) (bool, error) {
	resp, err := c.app.IsAdmin(ctx, &ssov1.IsAdminRequest{UserId: userID, AppId: appID})
	if err != nil {
		return false, err
	}
	return resp.GetIsAdmin(), nil
}

func (c *Client) GetUserInfo(ctx context.Context, userID uint32) (string, string, string, error) {
	resp, err := c.user.UserInfo(ctx, &ssov1.UserInfoRequest{UserId: userID})
	if err != nil {
		return "", "", "", err
	}
	return resp.GetEmail(), resp.GetSteamUrl(), resp.GetPathToPhoto(), nil
}

func (c *Client) GetUsers(ctx context.Context) (*ssov1.GetAllUsersResponse, error) {
	return c.user.GetAllUsers(ctx, &ssov1.GetAllUsersRequest{})
}

func (c *Client) GetUsersForApp(ctx context.Context, appID uint32) (*ssov1.GetAllUsersForAppResponse, error) {
	return c.app.GetAllUsersForApp(ctx, &ssov1.GetAllUsersForAppRequest{AppId: appID})
}

func (c *Client) UpdateUser(ctx context.Context, id uint32, email, password, steamURL, pathToPhoto string) (*ssov1.UpdateUserResponse, error) {
	return c.user.UpdateUser(ctx, &ssov1.UpdateUserRequest{
		Id:          id,
		Email:       email,
		Password:    password,
		SteamUrl:    steamURL,
		PathToPhoto: pathToPhoto,
	})
}

func (c *Client) DeleteUser(ctx context.Context, user *ssov1.DeleteUserRequest) (*ssov1.DeleteUserResponse, error) {
	return c.user.DeleteUser(ctx, user)
}
