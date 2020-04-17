// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/mattermost/matterbuild/server (interfaces: GithubSearchService)

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	gomock "github.com/golang/mock/gomock"
	github "github.com/google/go-github/github"
	reflect "reflect"
)

// MockGithubSearchService is a mock of GithubSearchService interface
type MockGithubSearchService struct {
	ctrl     *gomock.Controller
	recorder *MockGithubSearchServiceMockRecorder
}

// MockGithubSearchServiceMockRecorder is the mock recorder for MockGithubSearchService
type MockGithubSearchServiceMockRecorder struct {
	mock *MockGithubSearchService
}

// NewMockGithubSearchService creates a new mock instance
func NewMockGithubSearchService(ctrl *gomock.Controller) *MockGithubSearchService {
	mock := &MockGithubSearchService{ctrl: ctrl}
	mock.recorder = &MockGithubSearchServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockGithubSearchService) EXPECT() *MockGithubSearchServiceMockRecorder {
	return m.recorder
}

// Repositories mocks base method
func (m *MockGithubSearchService) Repositories(arg0 context.Context, arg1 string, arg2 *github.SearchOptions) (*github.RepositoriesSearchResult, *github.Response, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Repositories", arg0, arg1, arg2)
	ret0, _ := ret[0].(*github.RepositoriesSearchResult)
	ret1, _ := ret[1].(*github.Response)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// Repositories indicates an expected call of Repositories
func (mr *MockGithubSearchServiceMockRecorder) Repositories(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Repositories", reflect.TypeOf((*MockGithubSearchService)(nil).Repositories), arg0, arg1, arg2)
}
