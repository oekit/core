package validator

import (
	"testing"
)

type TestUser struct {
	Name  string `json:"name" validate:"required,min=1,max=100"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gte=0,lte=150"`
}

func TestValidate_Success(t *testing.T) {
	user := TestUser{Name: "test", Email: "test@example.com", Age: 25}
	if err := Validate(&user); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidate_Required(t *testing.T) {
	user := TestUser{Name: "", Email: "test@example.com", Age: 25}
	err := Validate(&user)
	if err == nil {
		t.Fatal("expected validation error")
	}

	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatal("expected ValidationError")
	}
	if ve.Len() == 0 {
		t.Fatal("expected at least one field error")
	}
	if ve.Errors()[0].Field != "name" {
		t.Errorf("expected field 'name', got '%s'", ve.Errors()[0].Field)
	}
}

func TestValidate_Email(t *testing.T) {
	user := TestUser{Name: "test", Email: "invalid", Age: 25}
	err := Validate(&user)
	if err == nil {
		t.Fatal("expected validation error")
	}

	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatal("expected ValidationError")
	}
	if ve.Errors()[0].Tag != "email" {
		t.Errorf("expected tag 'email', got '%s'", ve.Errors()[0].Tag)
	}
}

func TestValidate_Range(t *testing.T) {
	user := TestUser{Name: "test", Email: "test@example.com", Age: 200}
	err := Validate(&user)
	if err == nil {
		t.Fatal("expected validation error")
	}

	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatal("expected ValidationError")
	}
	if ve.Errors()[0].Tag != "lte" {
		t.Errorf("expected tag 'lte', got '%s'", ve.Errors()[0].Tag)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	user := TestUser{Name: "", Email: "invalid", Age: 200}
	err := Validate(&user)
	if err == nil {
		t.Fatal("expected validation error")
	}

	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatal("expected ValidationError")
	}
	if ve.Len() < 2 {
		t.Errorf("expected at least 2 errors, got %d", ve.Len())
	}
}

func TestValidateVar(t *testing.T) {
	if err := ValidateVar("test@example.com", "required,email"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if err := ValidateVar("invalid", "email"); err == nil {
		t.Error("expected validation error for invalid email")
	}
}

func TestValidationError_Map(t *testing.T) {
	user := TestUser{Name: "", Email: "invalid"}
	err := Validate(&user)
	if err == nil {
		t.Fatal("expected validation error")
	}

	ve, _ := AsValidationError(err)
	m := ve.Map()
	if _, ok := m["name"]; !ok {
		t.Error("expected 'name' in map")
	}
}

func TestValidationError_String(t *testing.T) {
	user := TestUser{Name: "", Email: "test@example.com"}
	err := Validate(&user)
	if err == nil {
		t.Fatal("expected validation error")
	}

	ve, _ := AsValidationError(err)
	s := ve.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
}

func TestIsValidationError(t *testing.T) {
	user := TestUser{Name: ""}
	err := Validate(&user)
	if !IsValidationError(err) {
		t.Error("expected IsValidationError to return true")
	}
}

func TestRegisterValidation(t *testing.T) {
	err := RegisterValidation("not_empty", func(val any) bool {
		s, ok := val.(string)
		if !ok {
			return false
		}
		return len(s) > 0
	})
	if err != nil {
		t.Fatalf("failed to register validation: %v", err)
	}

	type TestStruct struct {
		Value string `validate:"not_empty"`
	}

	if err := Validate(&TestStruct{Value: "test"}); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if err := Validate(&TestStruct{Value: ""}); err == nil {
		t.Error("expected validation error for empty value")
	}
}
