package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"games_webapp/internal/controllers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockGRPCClient struct {
	mock.Mock
}

func (m *MockGRPCClient) Login(ctx context.Context, email, password string, appID int32) (string, error) {
	args := m.Called(ctx, email, password)
	return args.String(0), args.Error(1)
}

func (m *MockGRPCClient) Register(ctx context.Context, email, password, steamURL, pathToPhoto string) (int64, error) {
	args := m.Called(ctx, email, password, steamURL, pathToPhoto)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockGRPCClient) GetUserInfo(ctx context.Context, userID int64) (email, steamURL, pathToPhoto string, err error) {
	args := m.Called(ctx, userID)
	return args.String(0), args.String(1), args.String(2), args.Error(3)
}

type MockUploads struct {
	mock.Mock
}

func (m *MockUploads) SaveImage(data []byte, filename string) error {
	args := m.Called(data, filename)
	return args.Error(0)
}

func (m *MockUploads) DeleteImage(filename string) error {
	args := m.Called(filename)
	return args.Error(0)
}

func (m *MockUploads) ReplaceImage(image []byte, oldFilename string) error {
	args := m.Called(image, oldFilename)
	return args.Error(0)
}

func TestAuthController_Register(t *testing.T) {
	tests := []struct {
		name         string
		requestBody  interface{}
		mockSetup    func(*MockGRPCClient, *MockUploads)
		expectedCode int
		expectedBody string
	}{
		{
			name: "successful registration",
			requestBody: map[string]interface{}{
				"email":     "test@example.com",
				"password":  "password123",
				"steam_url": "https://steam.com/user123",
				"photo":     []byte("test-photo-data"),
			},
			mockSetup: func(m *MockGRPCClient, u *MockUploads) {
				u.On("SaveImage", []byte("test-photo-data"), "test_example_com.jpg").Return(nil)
				m.On("Register", mock.Anything, "test@example.com", "password123", "https://steam.com/user123", "test_example_com.jpg").
					Return(int64(1), nil)
			},
			expectedCode: http.StatusOK,
			expectedBody: "1\n",
		},
		{
			name: "invalid JSON",
			requestBody: `{
				"email": "test@example.com",
				"password": "password123",
			}`, // trailing comma makes it invalid
			mockSetup:    func(m *MockGRPCClient, u *MockUploads) {},
			expectedCode: http.StatusBadRequest,
			expectedBody: "invalid JSON\n",
		},
		{
			name: "missing email",
			requestBody: map[string]interface{}{
				"password":  "password123",
				"steam_url": "https://steam.com/user123",
				"photo":     []byte("test-photo-data"),
			},
			mockSetup:    func(m *MockGRPCClient, u *MockUploads) {},
			expectedCode: http.StatusBadRequest,
			expectedBody: "missing email\n",
		},
		{
			name: "missing password",
			requestBody: map[string]interface{}{
				"email":     "test@example.com",
				"steam_url": "https://steam.com/user123",
				"photo":     []byte("test-photo-data"),
			},
			mockSetup:    func(m *MockGRPCClient, u *MockUploads) {},
			expectedCode: http.StatusBadRequest,
			expectedBody: "missing password\n",
		},
		{
			name: "missing steam url",
			requestBody: map[string]interface{}{
				"email":    "test@example.com",
				"password": "password123",
				"photo":    []byte("test-photo-data"),
			},
			mockSetup:    func(m *MockGRPCClient, u *MockUploads) {},
			expectedCode: http.StatusBadRequest,
			expectedBody: "missing steam url\n",
		},
		{
			name: "photo save error",
			requestBody: map[string]interface{}{
				"email":     "test@example.com",
				"password":  "password123",
				"steam_url": "https://steam.com/user123",
				"photo":     []byte("test-photo-data"),
			},
			mockSetup: func(m *MockGRPCClient, u *MockUploads) {
				u.On("SaveImage", []byte("test-photo-data"), "test_example_com.jpg").
					Return(errors.New("save failed"))
			},
			expectedCode: http.StatusInternalServerError,
			expectedBody: "failed to save photo\n",
		},
		{
			name: "registration error",
			requestBody: map[string]interface{}{
				"email":     "test@example.com",
				"password":  "password123",
				"steam_url": "https://steam.com/user123",
				"photo":     []byte("test-photo-data"),
			},
			mockSetup: func(m *MockGRPCClient, u *MockUploads) {
				u.On("SaveImage", []byte("test-photo-data"), "test_example_com.jpg").Return(nil)
				m.On("Register", mock.Anything, "test@example.com", "password123", "https://steam.com/user123", "test_example_com.jpg").
					Return(int64(0), errors.New("registration failed"))
			},
			expectedCode: http.StatusInternalServerError,
			expectedBody: "failed to register\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockGRPCClient)
			mockUploads := new(MockUploads)
			tt.mockSetup(mockClient, mockUploads)

			ctrl := controllers.NewAuthController(slog.Default(), mockClient, mockUploads)

			var bodyBytes []byte
			switch v := tt.requestBody.(type) {
			case string:
				bodyBytes = []byte(v)
			default:
				var err error
				bodyBytes, err = json.Marshal(v)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(bodyBytes))
			w := httptest.NewRecorder()

			ctrl.Register(w, req)

			assert.Equal(t, tt.expectedCode, w.Code)
			assert.Equal(t, tt.expectedBody, w.Body.String())
			mockClient.AssertExpectations(t)
			mockUploads.AssertExpectations(t)
		})
	}
}

func TestAuthController_Login(t *testing.T) {
	tests := []struct {
		name         string
		requestBody  interface{}
		mockSetup    func(*MockGRPCClient)
		expectedCode int
		expectedBody string
		checkCookie  bool
	}{
		{
			name: "successful login",
			requestBody: map[string]interface{}{
				"email":    "test@example.com",
				"password": "password123",
				"app_id":   1,
			},
			mockSetup: func(m *MockGRPCClient) {
				m.On("Login", mock.Anything, "test@example.com", "password123").
					Return("test_token", nil)
			},
			expectedCode: http.StatusOK,
			expectedBody: `"test_token"`,
			checkCookie:  true,
		},
		{
			name: "invalid JSON",
			requestBody: `{
				"email": "test@example.com",
				"password": "password123",
				"app_id": 1,
			}`, // trailing comma makes it invalid
			mockSetup:    func(m *MockGRPCClient) {},
			expectedCode: http.StatusBadRequest,
			expectedBody: "invalid JSON\n",
		},
		{
			name: "missing email",
			requestBody: map[string]interface{}{
				"password": "password123",
				"app_id":   1,
			},
			mockSetup:    func(m *MockGRPCClient) {},
			expectedCode: http.StatusBadRequest,
			expectedBody: "missing email or password or app id\n",
		},
		{
			name: "missing password",
			requestBody: map[string]interface{}{
				"email":  "test@example.com",
				"app_id": 1,
			},
			mockSetup:    func(m *MockGRPCClient) {},
			expectedCode: http.StatusBadRequest,
			expectedBody: "missing email or password or app id\n",
		},
		{
			name: "missing app_id",
			requestBody: map[string]interface{}{
				"email":    "test@example.com",
				"password": "password123",
			},
			mockSetup:    func(m *MockGRPCClient) {},
			expectedCode: http.StatusBadRequest,
			expectedBody: "missing email or password or app id\n",
		},
		{
			name: "login error",
			requestBody: map[string]interface{}{
				"email":    "test@example.com",
				"password": "password123",
				"app_id":   1,
			},
			mockSetup: func(m *MockGRPCClient) {
				m.On("Login", mock.Anything, "test@example.com", "password123").
					Return("", errors.New("login failed"))
			},
			expectedCode: http.StatusInternalServerError,
			expectedBody: "failed to login\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockGRPCClient)
			tt.mockSetup(mockClient)

			ctrl := controllers.NewAuthController(slog.Default(), mockClient, nil)

			var bodyBytes []byte
			switch v := tt.requestBody.(type) {
			case string:
				bodyBytes = []byte(v)
			default:
				var err error
				bodyBytes, err = json.Marshal(v)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(bodyBytes))
			w := httptest.NewRecorder()

			ctrl.Login(w, req)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == http.StatusOK {
				assert.JSONEq(t, tt.expectedBody, strings.TrimSpace(w.Body.String()))
			} else {
				assert.Equal(t, tt.expectedBody, w.Body.String())
			}

			if tt.checkCookie {
				cookies := w.Result().Cookies()
				assert.Len(t, cookies, 1)
				assert.Equal(t, "auth_token", cookies[0].Name)
				assert.Equal(t, "token", cookies[0].Value)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestAuthController_GetUserInfo(t *testing.T) {
	tests := []struct {
		name         string
		userID       interface{}
		mockSetup    func(*MockGRPCClient)
		expectedCode int
		expectedBody string
	}{
		{
			name:   "successful get user info",
			userID: int64(1),
			mockSetup: func(m *MockGRPCClient) {
				m.On("GetUserInfo", mock.Anything, int64(1)).
					Return("test@example.com", "https://steam.com/user123", "photo.jpg", nil)
			},
			expectedCode: http.StatusOK,
			expectedBody: `{"email":"test@example.com","steam_url":"https://steam.com/user123","photo":"photo.jpg"}`,
		},
		{
			name:         "unauthorized - no user_id in context",
			userID:       nil,
			mockSetup:    func(m *MockGRPCClient) {},
			expectedCode: http.StatusUnauthorized,
			expectedBody: "unauthorized\n",
		},
		{
			name:   "get user info error",
			userID: int64(1),
			mockSetup: func(m *MockGRPCClient) {
				m.On("GetUserInfo", mock.Anything, int64(1)).
					Return("", "", "", errors.New("get user info failed"))
			},
			expectedCode: http.StatusInternalServerError,
			expectedBody: "failed to get user info\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockGRPCClient)
			tt.mockSetup(mockClient)

			ctrl := controllers.NewAuthController(slog.Default(), mockClient, nil)

			req := httptest.NewRequest(http.MethodGet, "/userinfo", nil)
			if tt.userID != nil {
				ctx := context.WithValue(req.Context(), "user_id", tt.userID)
				req = req.WithContext(ctx)
			}

			w := httptest.NewRecorder()

			ctrl.GetUserInfo(w, req)

			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == http.StatusOK {
				assert.JSONEq(t, tt.expectedBody, strings.TrimSpace(w.Body.String()))
			} else {
				assert.Equal(t, tt.expectedBody, w.Body.String())
			}

			mockClient.AssertExpectations(t)
		})
	}
}
