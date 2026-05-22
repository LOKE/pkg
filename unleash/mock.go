package unleash

type MockClient struct {
	IsEnabledFunc func(name string, ctx Context) bool
}

func NewMockClient() *MockClient {
	return &MockClient{}
}

func (m *MockClient) IsEnabled(name string, ctx Context) bool {
	if m.IsEnabledFunc == nil {
		return false
	}
	return m.IsEnabledFunc(name, ctx)
}

func (m *MockClient) Close() error { return nil }
