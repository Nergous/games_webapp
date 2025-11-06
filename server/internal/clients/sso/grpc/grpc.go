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
	auth ssov1.AuthClient
	app  ssov1.AppClient
	user ssov1.UserClient
	log  *slog.Logger
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
		auth: ssov1.NewAuthClient(cc),
		app:  ssov1.NewAppClient(cc),
		user: ssov1.NewUserClient(cc),
		log:  log,
	}, nil
}

func InterceptorLogger(l *slog.Logger) grpclog.Logger {
	return grpclog.LoggerFunc(func(ctx context.Context, lvl grpclog.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(lvl), msg, fields...)
	})
}

func (c *Client) ValidateToken(ctx context.Context, token string) (uint32, bool, error) {
	resp, err := c.auth.ValidateToken(ctx, &ssov1.ValidateTokenRequest{Token: token})
	if err != nil {
		c.log.Error("sso.ValidateToken failed", slog.String("error", err.Error()))
		return 0, false, err
	}

	return resp.GetUserId(), resp.GetValid(), nil
}

func (c *Client) Register(ctx context.Context, email, password, steamURL, pathToPhoto string) (uint32, error) {
	resp, err := c.auth.Register(ctx, &ssov1.RegisterRequest{Email: email, Password: password, SteamUrl: steamURL, PathToPhoto: pathToPhoto})
	if err != nil {
		c.log.Error("sso.Register failed", slog.String("error", err.Error()))
		return 0, err
	}

	return resp.GetUserId(), nil
}

func (c *Client) Login(ctx context.Context, email, password string, appID uint32) (accessToken string, refreshToken string, err error) {
	resp, err := c.auth.Login(ctx, &ssov1.LoginRequest{Email: email, Password: password, AppId: appID})
	if err != nil {
		c.log.Error("sso.Login failed", slog.String("error", err.Error()))
		return "", "", err
	}

	return resp.GetAccessToken(), resp.GetRefreshToken(), nil
}

func (c *Client) Logout(ctx context.Context, refreshToken string) error {
	_, err := c.auth.Logout(ctx, &ssov1.LogoutRequest{Token: refreshToken})
	if err != nil {
		c.log.Error("sso.Logout failed", slog.String("error", err.Error()))
		return err
	}

	return nil
}

func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (accessToken, newRefreshToken string, err error) {
	resp, err := c.auth.Refresh(ctx, &ssov1.RefreshRequest{
		RefreshToken: refreshToken,
	})
	if err != nil {
		c.log.Error("sso.Refresh failed", slog.String("error", err.Error()))
		return "", "", err
	}

	return resp.GetAccessToken(), resp.GetRefreshToken(), nil
}

func (c *Client) IsAdmin(ctx context.Context, userID uint32, appID uint32) (bool, error) {
	resp, err := c.app.IsAdmin(ctx, &ssov1.IsAdminRequest{UserId: userID, AppId: appID})
	if err != nil {
		c.log.Error("sso.IsAdmin failed", slog.String("error", err.Error()))
		return false, err
	}

	return resp.GetIsAdmin(), nil
}

func (c *Client) GetUserInfo(ctx context.Context, userID uint32) (string, string, string, error) {
	resp, err := c.user.UserInfo(ctx, &ssov1.UserInfoRequest{UserId: userID})
	if err != nil {
		c.log.Error("sso.UserInfo failed", slog.String("error", err.Error()))
		return "", "", "", err
	}

	return resp.GetEmail(), resp.GetSteamUrl(), resp.GetPathToPhoto(), nil
}

func (c *Client) GetUsers(ctx context.Context) (*ssov1.GetAllUsersResponse, error) {
	resp, err := c.user.GetAllUsers(ctx, &ssov1.GetAllUsersRequest{})
	if err != nil {
		c.log.Error("sso.GetAllUser failed", slog.String("error", err.Error()))
		return nil, err
	}

	return resp, nil
}

func (c *Client) GetUsersForApp(ctx context.Context, appID uint32) (*ssov1.GetAllUsersForAppResponse, error) {
	resp, err := c.app.GetAllUsersForApp(ctx, &ssov1.GetAllUsersForAppRequest{AppId: appID})
	if err != nil {
		c.log.Error("sso.GetAllUser failed", slog.String("error", err.Error()))
		return nil, err
	}

	return resp, nil
}

func (c *Client) UpdateUser(ctx context.Context, user *ssov1.UpdateUserRequest) (*ssov1.UpdateUserResponse, error) {
	resp, err := c.user.UpdateUser(ctx, user)
	if err != nil {
		c.log.Error("sso.UpdateUser failed", slog.String("error", err.Error()))
		return nil, err
	}

	return resp, nil
}

func (c *Client) DeleteUser(ctx context.Context, user *ssov1.DeleteUserRequest) (*ssov1.DeleteUserResponse, error) {
	resp, err := c.user.DeleteUser(ctx, user)
	if err != nil {
		c.log.Error("sso.DeleteUser failed", slog.String("error", err.Error()))
		return nil, err
	}

	return resp, nil
}
