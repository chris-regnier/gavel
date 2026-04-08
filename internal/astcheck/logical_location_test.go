package astcheck

import (
	"testing"
)

// ---------------------------------------------------------------------------
// FindEnclosingFunction tests
// ---------------------------------------------------------------------------

func TestFindEnclosingFunction_Go(t *testing.T) {
	src := `package main

func topLevel() {
	x := 1
	_ = x
}

type Server struct{}

func (s *Server) Handle() {
	y := 2
	_ = y
}

func (s Server) Other() {
	z := 3
	_ = z
}
`
	tree := parseGo(t, src)
	source := []byte(src)

	tests := []struct {
		name      string
		line      int
		wantFunc  string
		wantClass string
		wantNil   bool
	}{
		{"top-level function body", 4, "topLevel", "", false},
		{"pointer receiver method body", 11, "Handle", "Server", false},
		{"value receiver method body", 16, "Other", "Server", false},
		{"outside any function (package decl)", 1, "", "", true},
		{"struct declaration line", 8, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := FindEnclosingFunction(tree.RootNode(), source, "go", tt.line)
			if tt.wantNil {
				if fc != nil {
					t.Fatalf("expected nil, got FuncName=%q ClassName=%q", fc.FuncName, fc.ClassName)
				}
				return
			}
			if fc == nil {
				t.Fatal("expected non-nil FunctionContext")
			}
			if fc.FuncName != tt.wantFunc {
				t.Errorf("FuncName = %q, want %q", fc.FuncName, tt.wantFunc)
			}
			if fc.ClassName != tt.wantClass {
				t.Errorf("ClassName = %q, want %q", fc.ClassName, tt.wantClass)
			}
		})
	}
}

func TestFindEnclosingFunction_Python(t *testing.T) {
	src := `class AuthService:
    def login(self, user):
        if user:
            return True
        return False

def standalone():
    pass
`
	tree := parsePython(t, src)
	source := []byte(src)

	tests := []struct {
		name      string
		line      int
		wantFunc  string
		wantClass string
		wantNil   bool
	}{
		{"method inside class", 3, "login", "AuthService", false},
		{"standalone function", 8, "standalone", "", false},
		{"class declaration line (outside method)", 1, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := FindEnclosingFunction(tree.RootNode(), source, "python", tt.line)
			if tt.wantNil {
				if fc != nil {
					t.Fatalf("expected nil, got FuncName=%q ClassName=%q", fc.FuncName, fc.ClassName)
				}
				return
			}
			if fc == nil {
				t.Fatal("expected non-nil FunctionContext")
			}
			if fc.FuncName != tt.wantFunc {
				t.Errorf("FuncName = %q, want %q", fc.FuncName, tt.wantFunc)
			}
			if fc.ClassName != tt.wantClass {
				t.Errorf("ClassName = %q, want %q", fc.ClassName, tt.wantClass)
			}
		})
	}
}

func TestFindEnclosingFunction_JavaScript(t *testing.T) {
	src := `class UserController {
  handleRequest(req) {
    return req.body;
  }
}

function freeFunction() {
  return 42;
}
`
	tree := parseJS(t, src)
	source := []byte(src)

	tests := []struct {
		name      string
		line      int
		wantFunc  string
		wantClass string
		wantNil   bool
	}{
		{"method inside class", 3, "handleRequest", "UserController", false},
		{"free function", 8, "freeFunction", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := FindEnclosingFunction(tree.RootNode(), source, "javascript", tt.line)
			if tt.wantNil {
				if fc != nil {
					t.Fatalf("expected nil, got FuncName=%q ClassName=%q", fc.FuncName, fc.ClassName)
				}
				return
			}
			if fc == nil {
				t.Fatal("expected non-nil FunctionContext")
			}
			if fc.FuncName != tt.wantFunc {
				t.Errorf("FuncName = %q, want %q", fc.FuncName, tt.wantFunc)
			}
			if fc.ClassName != tt.wantClass {
				t.Errorf("ClassName = %q, want %q", fc.ClassName, tt.wantClass)
			}
		})
	}
}

func TestFindEnclosingFunction_UnsupportedLanguage(t *testing.T) {
	src := `package main
func foo() {}
`
	tree := parseGo(t, src)
	fc := FindEnclosingFunction(tree.RootNode(), []byte(src), "unknown_lang", 2)
	if fc != nil {
		t.Fatalf("expected nil for unsupported language, got %+v", fc)
	}
}

// ---------------------------------------------------------------------------
// ResolveLogicalLocation tests
// ---------------------------------------------------------------------------

func TestResolveLogicalLocation_GoMethod(t *testing.T) {
	src := `package main

type Handler struct{}

func (h *Handler) ServeHTTP() {
	// finding here
}
`
	ll := ResolveLogicalLocation("server.go", []byte(src), 6)
	if ll == nil {
		t.Fatal("expected non-nil LogicalLocation")
	}
	if ll.Name != "ServeHTTP" {
		t.Errorf("Name = %q, want %q", ll.Name, "ServeHTTP")
	}
	if ll.Kind != "function" {
		t.Errorf("Kind = %q, want %q", ll.Kind, "function")
	}
	if ll.FullyQualifiedName != "Handler.ServeHTTP" {
		t.Errorf("FullyQualifiedName = %q, want %q", ll.FullyQualifiedName, "Handler.ServeHTTP")
	}
}

func TestResolveLogicalLocation_TopLevelFunction(t *testing.T) {
	src := `package main

func main() {
	fmt.Println("hello")
}
`
	ll := ResolveLogicalLocation("main.go", []byte(src), 4)
	if ll == nil {
		t.Fatal("expected non-nil LogicalLocation")
	}
	if ll.Name != "main" {
		t.Errorf("Name = %q, want %q", ll.Name, "main")
	}
	if ll.FullyQualifiedName != "main" {
		t.Errorf("FullyQualifiedName = %q, want %q", ll.FullyQualifiedName, "main")
	}
}

func TestResolveLogicalLocation_OutsideFunction(t *testing.T) {
	src := `package main

var x = 42
`
	ll := ResolveLogicalLocation("main.go", []byte(src), 3)
	if ll != nil {
		t.Fatalf("expected nil for line outside any function, got %+v", ll)
	}
}

func TestResolveLogicalLocation_UnsupportedExtension(t *testing.T) {
	ll := ResolveLogicalLocation("data.csv", []byte("a,b,c"), 1)
	if ll != nil {
		t.Fatalf("expected nil for unsupported file extension, got %+v", ll)
	}
}

func TestResolveLogicalLocationFromTree_ReusesTree(t *testing.T) {
	src := `package main

func helper() {
	x := 1
	_ = x
}
`
	tree := parseGo(t, src)
	ll := ResolveLogicalLocationFromTree(tree, []byte(src), "go", 4)
	if ll == nil {
		t.Fatal("expected non-nil LogicalLocation")
	}
	if ll.Name != "helper" {
		t.Errorf("Name = %q, want %q", ll.Name, "helper")
	}
}

// ---------------------------------------------------------------------------
// Nested function tests (innermost wins)
// ---------------------------------------------------------------------------

func TestFindEnclosingFunction_NestedPython(t *testing.T) {
	src := `def outer():
    def inner():
        x = 1
    return inner
`
	tree := parsePython(t, src)
	fc := FindEnclosingFunction(tree.RootNode(), []byte(src), "python", 3)
	if fc == nil {
		t.Fatal("expected non-nil FunctionContext")
	}
	if fc.FuncName != "inner" {
		t.Errorf("expected innermost function 'inner', got %q", fc.FuncName)
	}
}
