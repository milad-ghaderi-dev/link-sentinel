package service

import (
	"context"
	"errors"

	"github.com/milad-ghaderi-dev/link-sentinel/internal/auth"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

type UserRepository interface {
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	CreateUser(ctx context.Context, email, hashedPassword string) (int, error)
}

type AuthService struct {
	repo         UserRepository
	tokenManager *auth.TokenManager
}

func NewAuthService(repo UserRepository, token *auth.TokenManager) *AuthService {
	return &AuthService{
		repo:         repo,
		tokenManager: token,
	}
}

func (s *AuthService) Login(ctx context.Context, email, password string) (string, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return "", domain.ErrInvalidCredentials
		}
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", domain.ErrInvalidCredentials
	}

	token, err := s.tokenManager.GenerateToken(user.ID, user.Email)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (s *AuthService) Register(ctx context.Context, email, password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	userID, err := s.repo.CreateUser(ctx, email, string(hashedPassword))
	if err != nil {
		return "", err
	}

	token, err := s.tokenManager.GenerateToken(userID, email)
	if err != nil {
		return "", err
	}

	return token, nil
}
