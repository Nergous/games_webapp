package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"games_webapp/internal/controllers"
	"games_webapp/internal/middleware"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

// Test Suite Definition
type AuthControllerTestSuite struct {
	suite.Suite
	mockClient  *MockGRPCClient
	mockUploads *MockUploads
	controller  *controllers.AuthController
}

// Setup before each test
func (s *AuthControllerTestSuite) SetupTest() {
	s.mockClient = new(MockGRPCClient)
	s.mockUploads = new(MockUploads)
	s.controller = controllers.NewAuthController(slog.Default(), s.mockClient, s.mockUploads)
}

// Run the test suite
func TestAuthControllerSuite(t *testing.T) {
	suite.Run(t, new(AuthControllerTestSuite))
}

// Mock implementations (same as before)
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

// Helper functions
func (s *AuthControllerTestSuite) createMultipartRequest(formValues map[string]string, fileContent []byte) *http.Request {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for key, val := range formValues {
		err := writer.WriteField(key, val)
		s.Require().NoError(err)
	}

	if len(fileContent) > 0 {
		part, err := writer.CreateFormFile("image", "test.jpg")
		s.Require().NoError(err)
		_, err = part.Write(fileContent)
		s.Require().NoError(err)
	}

	s.Require().NoError(writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/register", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func (s *AuthControllerTestSuite) createJSONRequest(body interface{}) *http.Request {
	var bodyBytes []byte
	switch v := body.(type) {
	case string:
		bodyBytes = []byte(v)
	default:
		var err error
		bodyBytes, err = json.Marshal(v)
		s.Require().NoError(err)
	}

	return httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(bodyBytes))
}

// Test Cases
func (s *AuthControllerTestSuite) TestRegister_Success() {
	// Setup
	s.mockUploads.On("SaveImage", []byte("test-photo-data"), "test_example_com.jpg").Return(nil)
	s.mockClient.On("Register", mock.Anything, "test@example.com", "password123",
		"https://steam.com/user123", "test_example_com.jpg").Return(int64(1), nil)

	// Execute
	req := s.createMultipartRequest(map[string]string{
		"email":     "test@example.com",
		"password":  "password123",
		"steam_url": "https://steam.com/user123",
	}, []byte("test-photo-data"))
	w := httptest.NewRecorder()
	s.controller.Register(w, req)

	// Verify
	s.Equal(http.StatusOK, w.Code)
	s.Equal("1\n", w.Body.String())
	s.mockClient.AssertExpectations(s.T())
	s.mockUploads.AssertExpectations(s.T())
}

func (s *AuthControllerTestSuite) TestRegister_MissingEmail() {
	req := s.createMultipartRequest(map[string]string{
		"password":  "password123",
		"steam_url": "https://steam.com/user123",
	}, []byte("test-photo-data"))
	w := httptest.NewRecorder()
	s.controller.Register(w, req)

	s.Equal(http.StatusBadRequest, w.Code)
	s.Equal("missing email\n", w.Body.String())
}

func (s *AuthControllerTestSuite) TestRegister_MissingPassword() {
	req := s.createMultipartRequest(map[string]string{
		"email":     "test@example.com",
		"steam_url": "https://steam.com/user123",
	}, []byte("test-photo-data"))
	w := httptest.NewRecorder()
	s.controller.Register(w, req)

	s.Equal(http.StatusBadRequest, w.Code)
	s.Equal("missing password\n", w.Body.String())
}

func (s *AuthControllerTestSuite) TestRegister_ImageSaveError() {
	s.mockUploads.On("SaveImage", mock.Anything, mock.Anything).Return(errors.New("save failed"))

	req := s.createMultipartRequest(map[string]string{
		"email":     "test@example.com",
		"password":  "password123",
		"steam_url": "https://steam.com/user123",
	}, []byte("test-photo-data"))
	w := httptest.NewRecorder()
	s.controller.Register(w, req)

	s.Equal(http.StatusInternalServerError, w.Code)
	s.Equal("failed to save image\n", w.Body.String())
	s.mockUploads.AssertExpectations(s.T())
}

func (s *AuthControllerTestSuite) TestLogin_Success() {
	s.mockClient.On("Login", mock.Anything, "test@example.com", "password123").
		Return("test_token", nil)

	req := s.createJSONRequest(map[string]interface{}{
		"email":    "test@example.com",
		"password": "password123",
		"app_id":   1,
	})
	w := httptest.NewRecorder()
	s.controller.Login(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.JSONEq(`"test_token"`, strings.TrimSpace(w.Body.String()))

	cookies := w.Result().Cookies()
	s.Len(cookies, 1)
	s.Equal("auth_token", cookies[0].Name)
	s.Equal("token", cookies[0].Value)

	s.mockClient.AssertExpectations(s.T())
}

func (s *AuthControllerTestSuite) TestGetUserInfo_Success() {
	userID := int64(1)
	s.mockClient.On("GetUserInfo", mock.Anything, userID).
		Return("test@example.com", "https://steam.com/user123", "photo.jpg", nil)

	req := httptest.NewRequest(http.MethodGet, "/userinfo", nil)
	// Используем тот же ключ, что и в middleware (middleware.UserIDKey)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
	w := httptest.NewRecorder()

	s.controller.GetUserInfo(w, req)

	s.Equal(http.StatusOK, w.Code)
	s.JSONEq(`{
        "email": "test@example.com",
        "steam_url": "https://steam.com/user123",
        "photo": "photo.jpg"
    }`, w.Body.String())
	s.mockClient.AssertExpectations(s.T())
}
