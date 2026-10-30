package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateAndValidateToken_Valid(t *testing.T) {
	tm := NewTokenManager("test_secret_key_antigravity_123456")

	expectedUserID := 42
	expectedEmail := "developer@linky.dev"

	token, err := tm.GenerateToken(expectedUserID, expectedEmail)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	claims, err := tm.ValidateToken(token)
	if err != nil {
		t.Fatalf("unexpected error validating token: %v", err)
	}

	if claims.UserID != expectedUserID {
		t.Fatalf("expected userID %d, got %d", expectedUserID, claims.UserID)
	}

	if claims.Email != expectedEmail {
		t.Fatalf("expected email %s, got %s", expectedEmail, claims.Email)
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	tm1 := NewTokenManager("initial_secret_key_111")
	token, err := tm1.GenerateToken(10, "hacker@test.com")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	tm2 := NewTokenManager("different_secret_key_222")
	_, err = tm2.ValidateToken(token)
	if err == nil {
		t.Fatal("expected validation to fail with a different secret key, but it succeeded")
	}
}

func TestValidateToken_Malformed(t *testing.T) {
	tm := NewTokenManager("any_secret")

	malformedTokens := []string{
		"",
		"not-even-a-token",
		"header.payload",
		"header.payload.signature.extra",
	}

	for _, badToken := range malformedTokens {
		_, err := tm.ValidateToken(badToken)
		if err == nil {
			t.Errorf("expected error for malformed token %q, but got nil", badToken)
		}
	}
}

func TestValidateToken_Expired(t *testing.T) {
	secret := []byte("secret_for_expiry_test")
	tm := NewTokenManager(string(secret))

	claims := &Claims{
		UserID: 77,
		Email:  "expired@linky.dev",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	expiredToken, err := tokenObj.SignedString(secret)
	if err != nil {
		t.Fatalf("failed to create expired token: %v", err)
	}

	_, err = tm.ValidateToken(expiredToken)
	if err == nil {
		t.Fatal("expected expired token to fail validation, but it succeeded")
	}
}

func TestValidateToken_InvalidSigningMethod(t *testing.T) {
	tm := NewTokenManager("some_secret")

	// Token with None signing algorithm (unsigned)
	claims := &Claims{
		UserID: 99,
		Email:  "none_alg@linky.dev",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}
	tokenObj := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenString, err := tokenObj.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to create none-alg token: %v", err)
	}

	_, err = tm.ValidateToken(tokenString)
	if err == nil {
		t.Fatal("expected none-alg token to fail validation, but it succeeded")
	}
}
