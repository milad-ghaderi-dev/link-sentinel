package service

import (
	"context"
	"errors"
	"testing"

	"github.com/milad-ghaderi-dev/link-sentinel/internal/auth"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

type mockUserRepo struct {
	user *domain.User
	err  error
}

func (m *mockUserRepo) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return m.user, m.err
}

func (m *mockUserRepo) CreateUser(ctx context.Context, email, hashedPassword string) (int, error) {
	return 1, m.err
}

func TestAuthService_Login_Success(t *testing.T) {
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("correct_password"), bcrypt.DefaultCost)
	mockRepo := &mockUserRepo{
		user: &domain.User{
			ID:       10,
			Email:    "test@linky.dev",
			Password: string(hashedPassword),
		},
	}

	tm := auth.NewTokenManager("test_secret_key_1234567890")
	authSvc := NewAuthService(mockRepo, tm)

	token, err := authSvc.Login(context.Background(), "test@linky.dev", "correct_password")
	if err != nil {
		t.Fatalf("expected successful login, got error: %v", err)
	}

	claims, err := tm.ValidateToken(token)
	if err != nil {
		t.Fatalf("expected valid token, got: %v", err)
	}

	if claims.UserID != 10 || claims.Email != "test@linky.dev" {
		t.Fatalf("claims mismatch, got user_id=%d email=%s", claims.UserID, claims.Email)
	}
}

func TestAuthService_Login_UserNotFound(t *testing.T) {
	mockRepo := &mockUserRepo{
		err: domain.ErrUserNotFound,
	}

	tm := auth.NewTokenManager("test_secret_key")
	authSvc := NewAuthService(mockRepo, tm)

	_, err := authSvc.Login(context.Background(), "ghost@linky.dev", "any_password")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got: %v", err)
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("real_secret"), bcrypt.DefaultCost)
	mockRepo := &mockUserRepo{
		user: &domain.User{
			ID:       5,
			Email:    "user@linky.dev",
			Password: string(hashedPassword),
		},
	}

	tm := auth.NewTokenManager("test_secret_key")
	authSvc := NewAuthService(mockRepo, tm)

	_, err := authSvc.Login(context.Background(), "user@linky.dev", "wrong_password")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for wrong password, got: %v", err)
	}
}

func TestAuthService_Login_DatabaseError(t *testing.T) {
	expectedErr := errors.New("database connection refused")
	mockRepo := &mockUserRepo{
		err: expectedErr,
	}

	tm := auth.NewTokenManager("test_secret_key")
	authSvc := NewAuthService(mockRepo, tm)

	_, err := authSvc.Login(context.Background(), "user@linky.dev", "password")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected raw database error, got: %v", err)
	}
}

func TestAuthService_Register_Success(t *testing.T) {
	mockRepo := &mockUserRepo{}
	tm := auth.NewTokenManager("test_secret_for_register")
	authSvc := NewAuthService(mockRepo, tm)

	token, err := authSvc.Register(context.Background(), "newuser@link-sentinel.dev", "strong_password")
	if err != nil {
		t.Fatalf("expected successful registration, got: %v", err)
	}

	claims, err := tm.ValidateToken(token)
	if err != nil || claims.Email != "newuser@link-sentinel.dev" {
		t.Fatalf("expected valid token with correct email, got: %v", claims)
	}
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	mockRepo := &mockUserRepo{
		err: domain.ErrUserAlreadyExists,
	}
	tm := auth.NewTokenManager("test_secret_for_register")
	authSvc := NewAuthService(mockRepo, tm)

	_, err := authSvc.Register(context.Background(), "existing@link-sentinel.dev", "password123")
	if !errors.Is(err, domain.ErrUserAlreadyExists) {
		t.Fatalf("expected ErrUserAlreadyExists, got: %v", err)
	}
}
