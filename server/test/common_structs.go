package test

import "github.com/stretchr/testify/mock"

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
