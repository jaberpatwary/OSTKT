package usecase

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"

	"ticket-system/internal/domain"
)

const (
	RoleAdmin   = "admin"
	RoleManager = "manager"
	RoleAgent   = "agent"

	maximumPasswordBytes = 8 << 10
	argonMemoryKiB       = 64 * 1024
	argonIterations      = 3
	argonThreads         = 2
	argonSaltBytes       = 16
	argonKeyBytes        = 32
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidUser        = errors.New("invalid user details")
	ErrWeakPassword       = errors.New("password is required")
	ErrPasswordTooLong    = errors.New("password exceeds the 8 KB security limit")
	ErrLastAdmin          = domain.ErrLastAdmin
	ErrUsernameTaken      = domain.ErrDuplicateKey
	ErrInvalidSession     = errors.New("invalid or expired session")
	ErrMFAUnavailable     = errors.New("MFA encryption is not configured")
	ErrMFAAlreadyEnabled  = errors.New("MFA is already enabled")
	ErrMFAInvalidCode     = errors.New("authenticator code is invalid")
	usernamePattern       = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._@+-]{0,255}$`)
)

type SecurityService struct {
	repository domain.SecurityRepository
	mfaKey     []byte
}

func NewSecurityService(repository domain.SecurityRepository, encryptionKeys ...string) *SecurityService {
	service := &SecurityService{repository: repository}
	if len(encryptionKeys) > 0 && strings.TrimSpace(encryptionKeys[0]) != "" {
		key, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(encryptionKeys[0]))
		if err != nil {
			key, err = base64.StdEncoding.DecodeString(strings.TrimSpace(encryptionKeys[0]))
		}
		if err == nil && len(key) == 32 {
			service.mfaKey = key
		}
	}
	return service
}

func (s *SecurityService) BootstrapAdmin(ctx context.Context, username, password string) error {
	hasUsers, err := s.repository.HasUsers(ctx)
	if err != nil {
		return err
	}
	if hasUsers {
		return nil
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if !usernamePattern.MatchString(username) {
		return ErrInvalidUser
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	return s.repository.BootstrapAdmin(ctx, username, username, string(hash))
}

func (s *SecurityService) Authenticate(ctx context.Context, username, password string) (domain.User, error) {
	if len(password) == 0 || len(password) > maximumPasswordBytes {
		return domain.User{}, ErrInvalidCredentials
	}
	user, passwordHash, err := s.repository.FindUserByUsername(ctx, username)
	if err != nil || !user.Active || !verifyPassword(passwordHash, password) {
		return domain.User{}, ErrInvalidCredentials
	}
	return user, nil
}

func (s *SecurityService) CreateSession(ctx context.Context, username, password, otp string) (domain.SessionCredentials, error) {
	user, err := s.Authenticate(ctx, username, password)
	if err != nil {
		return domain.SessionCredentials{}, err
	}
	secretCiphertext, mfaEnabled, err := s.repository.GetMFA(ctx, user.ID)
	if err != nil {
		return domain.SessionCredentials{}, err
	}
	if mfaEnabled {
		secret, err := s.decryptMFASecret(secretCiphertext)
		if err != nil || !verifyTOTP(secret, otp, time.Now().UTC()) {
			return domain.SessionCredentials{}, ErrInvalidCredentials
		}
	}
	if err := s.repository.DeleteExpiredSessions(ctx, time.Now().UTC()); err != nil {
		return domain.SessionCredentials{}, err
	}
	token, err := randomSessionToken()
	if err != nil {
		return domain.SessionCredentials{}, err
	}
	csrf, err := randomSessionToken()
	if err != nil {
		return domain.SessionCredentials{}, err
	}
	expiresAt := time.Now().UTC().Add(12 * time.Hour)
	if err := s.repository.CreateSession(ctx, domain.LoginSession{
		UserID: user.ID, TokenHash: hashSessionToken(token), CSRFHash: hashSessionToken(csrf), ExpiresAt: expiresAt,
	}); err != nil {
		return domain.SessionCredentials{}, err
	}
	return domain.SessionCredentials{Token: token, CSRFToken: csrf, ExpiresAt: expiresAt, User: user}, nil
}

func (s *SecurityService) MFAStatus(ctx context.Context, userID uint) (bool, error) {
	_, enabled, err := s.repository.GetMFA(ctx, userID)
	return enabled, err
}

func (s *SecurityService) BeginMFASetup(ctx context.Context, user domain.User) (string, string, error) {
	if len(s.mfaKey) != 32 {
		return "", "", ErrMFAUnavailable
	}
	_, enabled, err := s.repository.GetMFA(ctx, user.ID)
	if err != nil {
		return "", "", err
	}
	if enabled {
		return "", "", ErrMFAAlreadyEnabled
	}
	secretBytes := make([]byte, 20)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", "", err
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secretBytes)
	ciphertext, err := s.encryptMFASecret(secret)
	if err != nil {
		return "", "", err
	}
	if err := s.repository.SetMFA(ctx, user.ID, ciphertext, false); err != nil {
		return "", "", err
	}
	issuer := "Auto Ticket System"
	label := url.PathEscape(issuer + ":" + user.Username)
	values := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return secret, "otpauth://totp/" + label + "?" + values.Encode(), nil
}

func (s *SecurityService) ConfirmMFASetup(ctx context.Context, user domain.User, code string) error {
	secretCiphertext, enabled, err := s.repository.GetMFA(ctx, user.ID)
	if err != nil {
		return err
	}
	if enabled {
		return ErrMFAAlreadyEnabled
	}
	secret, err := s.decryptMFASecret(secretCiphertext)
	if err != nil || !verifyTOTP(secret, code, time.Now().UTC()) {
		return ErrMFAInvalidCode
	}
	if err := s.repository.SetMFA(ctx, user.ID, secretCiphertext, true); err != nil {
		return err
	}
	return s.repository.DeleteSessionsForUser(ctx, user.ID)
}

func (s *SecurityService) DisableMFA(ctx context.Context, user domain.User, password, code string) error {
	verified, err := s.Authenticate(ctx, user.Username, password)
	if err != nil || verified.ID != user.ID {
		return ErrInvalidCredentials
	}
	secretCiphertext, enabled, err := s.repository.GetMFA(ctx, user.ID)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	secret, err := s.decryptMFASecret(secretCiphertext)
	if err != nil || !verifyTOTP(secret, code, time.Now().UTC()) {
		return ErrMFAInvalidCode
	}
	if err := s.repository.SetMFA(ctx, user.ID, "", false); err != nil {
		return err
	}
	return s.repository.DeleteSessionsForUser(ctx, user.ID)
}

func (s *SecurityService) encryptMFASecret(secret string) (string, error) {
	if len(s.mfaKey) != 32 {
		return "", ErrMFAUnavailable
	}
	block, err := aes.NewCipher(s.mfaKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(secret), nil)
	return base64.RawStdEncoding.EncodeToString(ciphertext), nil
}

func (s *SecurityService) decryptMFASecret(encoded string) (string, error) {
	if len(s.mfaKey) != 32 {
		return "", ErrMFAUnavailable
	}
	data, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.mfaKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(data) < gcm.NonceSize() {
		return "", ErrMFAUnavailable
	}
	secret, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	return string(secret), err
}

func verifyTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	for _, char := range code {
		if char < '0' || char > '9' {
			return false
		}
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return false
	}
	for skew := int64(-1); skew <= 1; skew++ {
		counter := uint64(now.Unix()/30 + skew)
		var message [8]byte
		binary.BigEndian.PutUint64(message[:], counter)
		mac := hmac.New(sha1.New, key)
		_, _ = mac.Write(message[:])
		digest := mac.Sum(nil)
		offset := digest[len(digest)-1] & 0x0f
		value := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff
		candidate := []byte(fmt.Sprintf("%06d", value%1000000))
		if subtle.ConstantTimeCompare(candidate, []byte(code)) == 1 {
			return true
		}
	}
	return false
}

func (s *SecurityService) AuthenticateSession(ctx context.Context, token, csrf string) (domain.User, error) {
	if len(token) < 40 || len(token) > 64 || len(csrf) < 40 || len(csrf) > 64 {
		return domain.User{}, ErrInvalidSession
	}
	session, err := s.repository.GetSession(ctx, hashSessionToken(token))
	if err != nil {
		return domain.User{}, ErrInvalidSession
	}
	if !session.ExpiresAt.After(time.Now().UTC()) {
		_ = s.repository.DeleteSession(ctx, session.TokenHash)
		return domain.User{}, ErrInvalidSession
	}
	csrfHash := hashSessionToken(csrf)
	if subtle.ConstantTimeCompare([]byte(csrfHash), []byte(session.CSRFHash)) != 1 {
		return domain.User{}, ErrInvalidSession
	}
	user, err := s.repository.GetUser(ctx, session.UserID)
	if err != nil || !user.Active {
		_ = s.repository.DeleteSession(ctx, session.TokenHash)
		return domain.User{}, ErrInvalidSession
	}
	return user, nil
}

func (s *SecurityService) RevokeSession(ctx context.Context, token string) error {
	if len(token) < 40 || len(token) > 64 {
		return ErrInvalidSession
	}
	return s.repository.DeleteSession(ctx, hashSessionToken(token))
}

func randomSessionToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func hashSessionToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (s *SecurityService) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.repository.ListUsers(ctx)
}

func (s *SecurityService) CreateUser(ctx context.Context, input domain.UserInput) (domain.User, error) {
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	if !usernamePattern.MatchString(input.Username) || input.DisplayName == "" || len([]rune(input.DisplayName)) > 256 || !validRole(input.Role) {
		return domain.User{}, ErrInvalidUser
	}
	if input.Password == "" {
		return domain.User{}, ErrWeakPassword
	}
	if len(input.Password) > maximumPasswordBytes {
		return domain.User{}, ErrPasswordTooLong
	}
	hash, err := hashPassword(input.Password)
	if err != nil {
		return domain.User{}, err
	}
	input.Active = true
	return s.repository.CreateUser(ctx, input, string(hash))
}

func (s *SecurityService) UpdateUser(ctx context.Context, id uint, input domain.UserUpdate) (domain.User, error) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	if input.DisplayName == "" || len([]rune(input.DisplayName)) > 256 || !validRole(input.Role) {
		return domain.User{}, ErrInvalidUser
	}
	var passwordHash string
	if input.Password != "" {
		if len(input.Password) > maximumPasswordBytes {
			return domain.User{}, ErrPasswordTooLong
		}
		if input.Password == "" {
			return domain.User{}, ErrWeakPassword
		}
		hash, err := hashPassword(input.Password)
		if err != nil {
			return domain.User{}, err
		}
		passwordHash = string(hash)
	}
	return s.repository.UpdateUser(ctx, id, input, passwordHash)
}

func (s *SecurityService) RecordAudit(ctx context.Context, event domain.AuditEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	return s.repository.CreateAudit(ctx, event)
}

func (s *SecurityService) ListAudit(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	return s.repository.ListAudit(ctx, limit)
}

func validRole(role string) bool {
	switch role {
	case RoleAdmin, RoleManager, RoleAgent:
		return true
	default:
		return false
	}
}

func hashPassword(password string) (string, error) {
	if len(password) == 0 {
		return "", ErrWeakPassword
	}
	if len(password) > maximumPasswordBytes {
		return "", ErrPasswordTooLong
	}
	salt := make([]byte, argonSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemoryKiB, argonThreads, argonKeyBytes)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemoryKiB, argonIterations, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func verifyPassword(encodedHash, password string) bool {
	if strings.HasPrefix(encodedHash, "$2") {
		return len(password) <= 72 && bcrypt.CompareHashAndPassword([]byte(encodedHash), []byte(password)) == nil
	}
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" ||
		parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", argonMemoryKiB, argonIterations, argonThreads) {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != argonSaltBytes {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) != argonKeyBytes {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, argonIterations, argonMemoryKiB, argonThreads, argonKeyBytes)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
