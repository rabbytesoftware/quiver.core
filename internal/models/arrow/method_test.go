package arrow

import "testing"

func TestAction_Validate(t *testing.T) {
	testCases := []struct {
		name      string
		action    Action
		expectErr bool
	}{
		{
			name: "valid run action",
			action: Action{
				Type:  ActionTypeRun,
				Value: "echo test",
				Name:  "Test action",
			},
			expectErr: false,
		},
		{
			name: "valid download action",
			action: Action{
				Type:  ActionTypeDownload,
				Value: "https://example.com/file.zip",
				To:    "/tmp",
				Name:  "Download",
			},
			expectErr: false,
		},
		{
			name: "invalid empty type",
			action: Action{
				Value: "test",
			},
			expectErr: true,
		},
		{
			name: "invalid empty value",
			action: Action{
				Type: ActionTypeRun,
			},
			expectErr: true,
		},
		{
			name: "invalid download without to",
			action: Action{
				Type:  ActionTypeDownload,
				Value: "https://example.com/file.zip",
			},
			expectErr: true,
		},
		{
			name: "invalid action type",
			action: Action{
				Type:  ActionType("invalid"),
				Value: "test",
			},
			expectErr: true,
		},
		{
			name: "valid copy action",
			action: Action{
				Type:  ActionTypeCopy,
				Value: "/source/file",
				To:    "/dest",
			},
			expectErr: false,
		},
		{
			name: "invalid copy without to",
			action: Action{
				Type:  ActionTypeCopy,
				Value: "/source/file",
			},
			expectErr: true,
		},
		{
			name: "valid uncompress action",
			action: Action{
				Type:  ActionTypeUncompress,
				Value: "file.tar.gz",
				To:    "/dest",
			},
			expectErr: false,
		},
		{
			name: "invalid uncompress without to",
			action: Action{
				Type:  ActionTypeUncompress,
				Value: "file.tar.gz",
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.action.Validate()
			if tc.expectErr && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestMethod_Validate(t *testing.T) {
	testCases := []struct {
		name      string
		method    Method
		expectErr bool
	}{
		{
			name: "valid method",
			method: Method{
				Platforms:  []string{"linux/amd64"},
				MethodName: "install",
				Actions: []Action{
					{
						Type:  ActionTypeRun,
						Value: "echo test",
					},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid no platforms",
			method: Method{
				MethodName: "install",
				Actions: []Action{
					{Type: ActionTypeRun, Value: "test"},
				},
			},
			expectErr: true,
		},
		{
			name: "invalid empty method name",
			method: Method{
				Platforms: []string{"linux/amd64"},
				Actions: []Action{
					{Type: ActionTypeRun, Value: "test"},
				},
			},
			expectErr: true,
		},
		{
			name: "invalid no actions",
			method: Method{
				Platforms:  []string{"linux/amd64"},
				MethodName: "install",
			},
			expectErr: true,
		},
		{
			name: "invalid action in array",
			method: Method{
				Platforms:  []string{"linux/amd64"},
				MethodName: "install",
				Actions: []Action{
					{Type: ActionTypeRun, Value: "valid"},
					{Type: ActionType(""), Value: "invalid"},
				},
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.method.Validate()
			if tc.expectErr && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}
