package browser

import (
	"testing"

	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"github.com/shhac/grotto/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestNewServiceBrowser(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	services := binding.NewUntypedList()
	browser := NewServiceBrowser(services)

	assert.NotNil(t, browser, "ServiceBrowser should not be nil")
	assert.NotNil(t, browser.tree, "tree should be initialized")
}

func TestServiceBrowser_DisplaysServices(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	// Create mock services
	services := binding.NewUntypedList()
	mockServices := []interface{}{
		domain.Service{
			Name:     "UserService",
			FullName: "example.UserService",
			Methods: []domain.Method{
				{
					Name:           "GetUser",
					FullName:       "example.UserService.GetUser",
					InputType:      "GetUserRequest",
					OutputType:     "GetUserResponse",
					IsClientStream: false,
					IsServerStream: false,
				},
				{
					Name:           "ListUsers",
					FullName:       "example.UserService.ListUsers",
					InputType:      "ListUsersRequest",
					OutputType:     "ListUsersResponse",
					IsClientStream: false,
					IsServerStream: true,
				},
			},
		},
		domain.Service{
			Name:     "ProductService",
			FullName: "example.ProductService",
			Methods: []domain.Method{
				{
					Name:           "CreateProduct",
					FullName:       "example.ProductService.CreateProduct",
					InputType:      "CreateProductRequest",
					OutputType:     "CreateProductResponse",
					IsClientStream: false,
					IsServerStream: false,
				},
			},
		},
	}

	for _, s := range mockServices {
		services.Append(s)
	}

	browser := NewServiceBrowser(services)

	// Test that services are available as root UIDs
	serviceUIDs := browser.getServiceUIDs()
	assert.Len(t, serviceUIDs, 2, "should have 2 services")
	assert.Contains(t, serviceUIDs, "example.UserService")
	assert.Contains(t, serviceUIDs, "example.ProductService")
}

func TestServiceBrowser_GetMethodUIDs(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	services := binding.NewUntypedList()
	mockService := domain.Service{
		Name:     "UserService",
		FullName: "example.UserService",
		Methods: []domain.Method{
			{
				Name:           "GetUser",
				FullName:       "example.UserService.GetUser",
				InputType:      "GetUserRequest",
				OutputType:     "GetUserResponse",
				IsClientStream: false,
				IsServerStream: false,
			},
			{
				Name:           "ListUsers",
				FullName:       "example.UserService.ListUsers",
				InputType:      "ListUsersRequest",
				OutputType:     "ListUsersResponse",
				IsClientStream: false,
				IsServerStream: true,
			},
		},
	}
	services.Append(mockService)

	browser := NewServiceBrowser(services)

	// Test that methods are returned for a service
	methodUIDs := browser.getMethodUIDs("example.UserService")
	assert.Len(t, methodUIDs, 2, "should have 2 methods")
	assert.Contains(t, methodUIDs, "example.UserService:GetUser")
	assert.Contains(t, methodUIDs, "example.UserService:ListUsers")
}

func TestServiceBrowser_IsBranch(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	services := binding.NewUntypedList()
	browser := NewServiceBrowser(services)

	tests := []struct {
		name     string
		uid      string
		expected bool
	}{
		{
			name:     "service is branch",
			uid:      "example.UserService",
			expected: true,
		},
		{
			name:     "method is leaf",
			uid:      "example.UserService:GetUser",
			expected: false,
		},
		{
			name:     "empty is branch",
			uid:      "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := browser.isBranch(tt.uid)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestServiceBrowser_FindService(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	services := binding.NewUntypedList()
	mockService := domain.Service{
		Name:     "UserService",
		FullName: "example.UserService",
		Methods: []domain.Method{
			{
				Name:      "GetUser",
				FullName:  "example.UserService.GetUser",
				InputType: "GetUserRequest",
			},
		},
	}
	services.Append(mockService)

	browser := NewServiceBrowser(services)

	// Test finding existing service
	found := browser.findService("example.UserService")
	assert.NotNil(t, found, "should find service")
	assert.Equal(t, "UserService", found.Name)
	assert.Equal(t, "example.UserService", found.FullName)

	// Test not finding non-existent service
	notFound := browser.findService("example.NonExistentService")
	assert.Nil(t, notFound, "should not find non-existent service")
}

func TestServiceBrowser_FindMethod(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	services := binding.NewUntypedList()
	mockService := domain.Service{
		Name:     "UserService",
		FullName: "example.UserService",
		Methods: []domain.Method{
			{
				Name:           "GetUser",
				FullName:       "example.UserService.GetUser",
				InputType:      "GetUserRequest",
				OutputType:     "GetUserResponse",
				IsClientStream: false,
				IsServerStream: false,
			},
			{
				Name:           "ListUsers",
				FullName:       "example.UserService.ListUsers",
				InputType:      "ListUsersRequest",
				OutputType:     "ListUsersResponse",
				IsClientStream: false,
				IsServerStream: true,
			},
		},
	}
	services.Append(mockService)

	browser := NewServiceBrowser(services)

	// Test finding existing method
	found := browser.findMethod(mockService, "GetUser")
	assert.NotNil(t, found, "should find method")
	assert.Equal(t, "GetUser", found.Name)
	assert.Equal(t, "GetUserRequest", found.InputType)

	// Test not finding non-existent method
	notFound := browser.findMethod(mockService, "NonExistentMethod")
	assert.Nil(t, notFound, "should not find non-existent method")
}

func TestServiceBrowser_OnMethodSelect(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	services := binding.NewUntypedList()
	mockService := domain.Service{
		Name:     "UserService",
		FullName: "example.UserService",
		Methods: []domain.Method{
			{
				Name:           "GetUser",
				FullName:       "example.UserService.GetUser",
				InputType:      "GetUserRequest",
				OutputType:     "GetUserResponse",
				IsClientStream: false,
				IsServerStream: false,
			},
		},
	}
	services.Append(mockService)

	browser := NewServiceBrowser(services)

	// Set up callback to capture selected method
	var selectedService domain.Service
	var selectedMethod domain.Method
	callbackCalled := false

	browser.SetOnMethodSelect(func(service domain.Service, method domain.Method) {
		selectedService = service
		selectedMethod = method
		callbackCalled = true
	})

	// Simulate selecting a method
	browser.onTreeSelected("example.UserService:GetUser")

	assert.True(t, callbackCalled, "callback should be called")
	assert.Equal(t, "UserService", selectedService.Name)
	assert.Equal(t, "GetUser", selectedMethod.Name)
}

func TestServiceBrowser_GetMethodIcon(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	services := binding.NewUntypedList()
	browser := NewServiceBrowser(services)

	tests := []struct {
		name         string
		method       domain.Method
		expectedType string // We can't compare resources directly, so we describe what we expect
	}{
		{
			name: "unary method",
			method: domain.Method{
				Name:           "GetUser",
				IsClientStream: false,
				IsServerStream: false,
			},
			expectedType: "unary",
		},
		{
			name: "client stream method",
			method: domain.Method{
				Name:           "UploadData",
				IsClientStream: true,
				IsServerStream: false,
			},
			expectedType: "client_stream",
		},
		{
			name: "server stream method",
			method: domain.Method{
				Name:           "ListItems",
				IsClientStream: false,
				IsServerStream: true,
			},
			expectedType: "server_stream",
		},
		{
			name: "bidi stream method",
			method: domain.Method{
				Name:           "Chat",
				IsClientStream: true,
				IsServerStream: true,
			},
			expectedType: "bidi_stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			icon := browser.getMethodIcon(&tt.method)
			assert.NotNil(t, icon, "icon should not be nil")
			// We just verify the icon is returned; actual icon comparison is difficult
		})
	}
}

func TestServiceBrowser_GetMethodTypeBadge(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	services := binding.NewUntypedList()
	browser := NewServiceBrowser(services)

	tests := []struct {
		name     string
		method   domain.Method
		expected string
	}{
		{
			name: "unary method",
			method: domain.Method{
				Name:           "GetUser",
				IsClientStream: false,
				IsServerStream: false,
			},
			expected: "",
		},
		{
			name: "client stream method",
			method: domain.Method{
				Name:           "UploadData",
				IsClientStream: true,
				IsServerStream: false,
			},
			expected: "↑",
		},
		{
			name: "server stream method",
			method: domain.Method{
				Name:           "ListItems",
				IsClientStream: false,
				IsServerStream: true,
			},
			expected: "↓",
		},
		{
			name: "bidi stream method",
			method: domain.Method{
				Name:           "Chat",
				IsClientStream: true,
				IsServerStream: true,
			},
			expected: "⇅",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			badge := browser.getMethodTypeBadge(&tt.method)
			assert.Equal(t, tt.expected, badge)
		})
	}
}

func TestServiceBrowser_ChildUIDs(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	services := binding.NewUntypedList()
	mockService := domain.Service{
		Name:     "UserService",
		FullName: "example.UserService",
		Methods: []domain.Method{
			{
				Name:     "GetUser",
				FullName: "example.UserService.GetUser",
			},
		},
	}
	services.Append(mockService)

	browser := NewServiceBrowser(services)

	// Test root level (empty UID)
	rootChildren := browser.childUIDs("")
	assert.Len(t, rootChildren, 1, "should have 1 service")
	assert.Contains(t, rootChildren, "example.UserService")

	// Test service level
	serviceChildren := browser.childUIDs("example.UserService")
	assert.Len(t, serviceChildren, 1, "should have 1 method")
	assert.Contains(t, serviceChildren, "example.UserService:GetUser")

	// Test method level (leaf - no children)
	methodChildren := browser.childUIDs("example.UserService:GetUser")
	assert.Len(t, methodChildren, 0, "methods should have no children")
}

func TestServiceBrowser_Refresh(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	services := binding.NewUntypedList()
	browser := NewServiceBrowser(services)

	// Add a service after creating the browser
	mockService := domain.Service{
		Name:     "UserService",
		FullName: "example.UserService",
		Methods:  []domain.Method{},
	}
	services.Append(mockService)

	// Refresh should not panic
	assert.NotPanics(t, func() {
		browser.Refresh()
	})
}

func TestServiceBrowser_ErrorService(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	services := binding.NewUntypedList()
	normalService := domain.Service{
		Name:     "HealthService",
		FullName: "example.HealthService",
		Methods: []domain.Method{
			{Name: "Check", FullName: "example.HealthService.Check"},
		},
	}
	errorService := domain.Service{
		Name:     "BrokenService",
		FullName: "example.BrokenService",
		Error:    "failed to resolve type: some.Missing.Type",
	}

	services.Append(normalService)
	services.Append(errorService)

	browser := NewServiceBrowser(services)

	// Both services should appear in the tree
	serviceUIDs := browser.getServiceUIDs()
	assert.Len(t, serviceUIDs, 2)
	assert.Contains(t, serviceUIDs, "example.HealthService")
	assert.Contains(t, serviceUIDs, "example.BrokenService")

	// Normal service has method children
	healthMethods := browser.getMethodUIDs("example.HealthService")
	assert.Len(t, healthMethods, 1)

	// Error service has no method children
	brokenMethods := browser.getMethodUIDs("example.BrokenService")
	assert.Len(t, brokenMethods, 0)

	// Both are branches
	assert.True(t, browser.isBranch("example.HealthService"))
	assert.True(t, browser.isBranch("example.BrokenService"))
}

func TestServiceBrowser_SortedAlphabetically(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	services := binding.NewUntypedList()
	// Add services in non-alphabetical order
	services.Append(domain.Service{
		Name: "ZebraService", FullName: "example.ZebraService",
		Methods: []domain.Method{
			{Name: "Zap", FullName: "example.ZebraService.Zap"},
			{Name: "Alpha", FullName: "example.ZebraService.Alpha"},
			{Name: "Middle", FullName: "example.ZebraService.Middle"},
		},
	})
	services.Append(domain.Service{
		Name: "AlphaService", FullName: "example.AlphaService",
		Methods: []domain.Method{
			{Name: "Do", FullName: "example.AlphaService.Do"},
		},
	})
	services.Append(domain.Service{
		Name: "MiddleService", FullName: "example.MiddleService",
		Methods: []domain.Method{
			{Name: "Run", FullName: "example.MiddleService.Run"},
		},
	})

	browser := NewServiceBrowser(services)

	// Services should be sorted alphabetically by full name
	serviceUIDs := browser.getServiceUIDs()
	assert.Equal(t, []string{
		"example.AlphaService",
		"example.MiddleService",
		"example.ZebraService",
	}, serviceUIDs)

	// Methods within a service should also be sorted
	methodUIDs := browser.getMethodUIDs("example.ZebraService")
	assert.Equal(t, []string{
		"example.ZebraService:Alpha",
		"example.ZebraService:Middle",
		"example.ZebraService:Zap",
	}, methodUIDs)
}

func TestBuildDisplayNames_NoCollision(t *testing.T) {
	index := map[string]domain.Service{
		"com.example.api.UserService":    {Name: "UserService", FullName: "com.example.api.UserService"},
		"com.example.api.ProductService": {Name: "ProductService", FullName: "com.example.api.ProductService"},
	}
	display := buildDisplayNames(index)
	assert.Equal(t, "UserService", display["com.example.api.UserService"])
	assert.Equal(t, "ProductService", display["com.example.api.ProductService"])
}

func TestBuildDisplayNames_WithCollision(t *testing.T) {
	index := map[string]domain.Service{
		"com.foo.UserService": {Name: "UserService", FullName: "com.foo.UserService"},
		"com.bar.UserService": {Name: "UserService", FullName: "com.bar.UserService"},
	}
	display := buildDisplayNames(index)
	assert.Equal(t, "foo.UserService", display["com.foo.UserService"])
	assert.Equal(t, "bar.UserService", display["com.bar.UserService"])
}

func TestBuildDisplayNames_DeepCollision(t *testing.T) {
	index := map[string]domain.Service{
		"com.example.v1.UserService": {Name: "UserService", FullName: "com.example.v1.UserService"},
		"com.example.v2.UserService": {Name: "UserService", FullName: "com.example.v2.UserService"},
		"org.other.UserService":      {Name: "UserService", FullName: "org.other.UserService"},
	}
	display := buildDisplayNames(index)
	// v1 and v2 share "example" so need one more segment
	assert.Equal(t, "v1.UserService", display["com.example.v1.UserService"])
	assert.Equal(t, "v2.UserService", display["com.example.v2.UserService"])
	assert.Equal(t, "other.UserService", display["org.other.UserService"])
}

func TestServiceBrowser_OnServiceError(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	services := binding.NewUntypedList()
	errorService := domain.Service{
		Name:     "BrokenService",
		FullName: "example.BrokenService",
		Error:    "unresolvable type dependency",
	}
	services.Append(errorService)

	browser := NewServiceBrowser(services)

	var capturedService domain.Service
	callbackCalled := false
	browser.SetOnServiceError(func(service domain.Service) {
		capturedService = service
		callbackCalled = true
	})

	// Selecting an error service should trigger onServiceError, not expand/collapse
	browser.onTreeSelected("example.BrokenService")

	assert.True(t, callbackCalled, "onServiceError callback should be called")
	assert.Equal(t, "BrokenService", capturedService.Name)
	assert.Equal(t, "unresolvable type dependency", capturedService.Error)
}
