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

func (c *Client) Login(ctx context.Context, email, password string, appID int32) (accessToken string, refreshToken string, err error) {
	resp, err := c.api.Login(ctx, &ssov1.LoginRequest{Email: email, Password: password, AppId: appID})
	if err != nil {
		c.log.Error("sso.Login failed", slog.String("error", err.Error()))
		return "", "", err
	}

	return resp.GetAccessToken(), resp.GetRefreshToken(), nil
}

func (c *Client) Logout(ctx context.Context, refreshToken string) error {
	_, err := c.api.Logout(ctx, &ssov1.LogoutRequest{Token: refreshToken})
	if err != nil {
		c.log.Error("sso.Logout failed", slog.String("error", err.Error()))
		return err
	}

	return nil
}

func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (accessToken, newRefreshToken string, err error) {
	resp, err := c.api.Refresh(ctx, &ssov1.RefreshRequest{
		RefreshToken: refreshToken,
	})
	if err != nil {
		c.log.Error("sso.Refresh failed", slog.String("error", err.Error()))
		return "", "", err
	}

	return resp.GetAccessToken(), resp.GetRefreshToken(), nil
}

func (c *Client) IsAdmin(ctx context.Context, userID int64) (bool, error) {
	resp, err := c.api.IsAdmin(ctx, &ssov1.IsAdminRequest{UserId: userID})
	if err != nil {
		c.log.Error("sso.IsAdmin failed", slog.String("error", err.Error()))
		return false, err
	}

	return resp.GetIsAdmin(), nil
}

func (c *Client) GetUserInfo(ctx context.Context, userID int64) (string, string, string, error) {
	resp, err := c.api.UserInfo(ctx, &ssov1.UserInfoRequest{UserId: userID})
	if err != nil {
		c.log.Error("sso.UserInfo failed", slog.String("error", err.Error()))
		return "", "", "", err
	}

	return resp.GetEmail(), resp.GetSteamUrl(), resp.GetPathToPhoto(), nil
}

func (c *Client) GetUsers(ctx context.Context) (*ssov1.GetAllUsersResponse, error) {
	resp, err := c.api.GetAllUsers(ctx, &ssov1.GetAllUsersRequest{})
	if err != nil {
		c.log.Error("sso.GetAllUser failed", slog.String("error", err.Error()))
		return nil, err
	}

	return resp, nil
}

func (c *Client) UpdateUser(ctx context.Context, user *ssov1.UpdateUserRequest) (*ssov1.UpdateUserResponse, error) {
	resp, err := c.api.UpdateUser(ctx, user)
	if err != nil {
		c.log.Error("sso.UpdateUser failed", slog.String("error", err.Error()))
		return nil, err
	}

	return resp, nil
}

func (c *Client) DeleteUser(ctx context.Context, user *ssov1.DeleteUserRequest) (*ssov1.DeleteUserResponse, error) {
	resp, err := c.api.DeleteUser(ctx, user)
	if err != nil {
		c.log.Error("sso.DeleteUser failed", slog.String("error", err.Error()))
		return nil, err
	}

	return resp, nil
}
