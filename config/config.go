package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config menampung seluruh konfigurasi aplikasi yang dibaca dari environment.
type Config struct {
	App           AppConfig
	DB            DBConfig
	Redis         RedisConfig
	Mongo         MongoConfig
	Auth          AuthConfig
	Business      BusinessConfig
	Worker        WorkerConfig
	Observability ObservabilityConfig
}

type AppConfig struct {
	Env              string
	Name             string
	Port             int
	BaseURL          string
	RequestTimeout   time.Duration
	TrustedProxies   string
	CORSAllowOrigins string
	LogLevel         string
}

type DBConfig struct {
	Host             string
	Port             string
	User             string
	Password         string
	Name             string
	SSLMode          string
	MaxConns         int32
	MinConns         int32
	MaxConnLifetime  time.Duration
	MaxConnIdleTime  time.Duration
	ConnectTimeout   time.Duration
	StatementTimeout time.Duration
	LockTimeout      time.Duration
	TxDeadline       time.Duration
	RetryMax         int
}

type RedisConfig struct {
	AdmissionAddr     string
	AdmissionPassword string
	AdmissionDB       int
	QueueAddr         string
	QueuePassword     string
	QueueDB           int
	AsynqConcurrency  int
}

type MongoConfig struct {
	URI            string
	Database       string
	ConnectTimeout time.Duration
}

type AuthConfig struct {
	Issuer               string
	Audience             string
	AccessSecret         string
	AccessTTL            time.Duration
	RefreshTTL           time.Duration
	AdmissionSecret      string
	AdmissionTTL         time.Duration
	TicketTokenEncKey    string
	BcryptCost           int
	LoginRateLimitPerMin int
}

type BusinessConfig struct {
	HoldTTL                time.Duration
	ReleaseGrace           time.Duration
	MaxTicketsPerUser      int
	MaxSwapPerHold         int
	AdmissionPassesPerSec  int
	ReserveRatePerUser     int
	ReserveBurstPerUser    int
	QueueEntryIdleTimeout  time.Duration
	QueueHeartbeatInterval time.Duration
}

type WorkerConfig struct {
	OutboxRelayInterval   time.Duration
	OutboxLease           time.Duration
	ExpiryScanInterval    time.Duration
	PaymentReconcileEvery time.Duration
}

type ObservabilityConfig struct {
	MetricsEnabled bool
	MetricsPort    int
	OTLPEndpoint   string
}

// Load membaca file .env bila ada, lalu mengambil nilai dari environment.
func Load() (Config, error) {
	_ = godotenv.Load()

	var cfg Config
	var errs []error

	cfg.App = AppConfig{
		Env:              envString("APP_ENV", "development"),
		Name:             envString("APP_NAME", "ngawimusical-backend"),
		BaseURL:          envString("APP_BASE_URL", "http://127.0.0.1:8080"),
		TrustedProxies:   envString("APP_TRUSTED_PROXIES", ""),
		CORSAllowOrigins: envString("APP_CORS_ALLOWED_ORIGINS", ""),
		LogLevel:         envString("LOG_LEVEL", "debug"),
	}

	var err error
	if cfg.App.Port, err = envInt("APP_PORT", 8080); err != nil {
		errs = append(errs, err)
	}
	if cfg.App.RequestTimeout, err = envDuration("APP_REQUEST_TIMEOUT", 3*time.Second); err != nil {
		errs = append(errs, err)
	}

	cfg.DB = DBConfig{
		Host:     envString("DB_HOST", "127.0.0.1"),
		Port:     envString("DB_PORT", "5432"),
		User:     envString("DB_USER", "ngawimusical"),
		Password: envString("DB_PASSWORD", "ngawimusical"),
		Name:     envString("DB_NAME", "ngawimusical"),
		SSLMode:  envString("DB_SSLMODE", "disable"),
	}

	maxConns, err := envInt("DB_MAX_CONNS", 60)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.DB.MaxConns = int32(maxConns)

	minConns, err := envInt("DB_MIN_CONNS", 5)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.DB.MinConns = int32(minConns)

	if cfg.DB.MaxConnLifetime, err = envDuration("DB_MAX_CONN_LIFETIME", time.Hour); err != nil {
		errs = append(errs, err)
	}
	if cfg.DB.MaxConnIdleTime, err = envDuration("DB_MAX_CONN_IDLE_TIME", 5*time.Minute); err != nil {
		errs = append(errs, err)
	}
	if cfg.DB.ConnectTimeout, err = envDuration("DB_CONNECT_TIMEOUT", 5*time.Second); err != nil {
		errs = append(errs, err)
	}
	if cfg.DB.StatementTimeout, err = envDuration("DB_STATEMENT_TIMEOUT", time.Second); err != nil {
		errs = append(errs, err)
	}
	if cfg.DB.LockTimeout, err = envDuration("DB_LOCK_TIMEOUT", 250*time.Millisecond); err != nil {
		errs = append(errs, err)
	}
	if cfg.DB.TxDeadline, err = envDuration("DB_TX_DEADLINE", 3*time.Second); err != nil {
		errs = append(errs, err)
	}
	if cfg.DB.RetryMax, err = envInt("DB_RETRY_MAX", 2); err != nil {
		errs = append(errs, err)
	}
	if cfg.DB.TxDeadline, err = envDuration("DB_TX_DEADLINE", 3*time.Second); err != nil {
		errs = append(errs, err)
	}

	cfg.Redis = RedisConfig{
		AdmissionAddr:     envString("REDIS_ADMISSION_ADDR", "127.0.0.1:6379"),
		AdmissionPassword: envString("REDIS_ADMISSION_PASSWORD", ""),
		QueueAddr:         envString("REDIS_QUEUE_ADDR", "127.0.0.1:6380"),
		QueuePassword:     envString("REDIS_QUEUE_PASSWORD", ""),
	}
	if cfg.Redis.AdmissionDB, err = envInt("REDIS_ADMISSION_DB", 0); err != nil {
		errs = append(errs, err)
	}
	if cfg.Redis.QueueDB, err = envInt("REDIS_QUEUE_DB", 0); err != nil {
		errs = append(errs, err)
	}
	if cfg.Redis.AsynqConcurrency, err = envInt("ASYNQ_CONCURRENCY", 10); err != nil {
		errs = append(errs, err)
	}

	cfg.Mongo = MongoConfig{
		URI:      envString("MONGO_URI", "mongodb://127.0.0.1:27017"),
		Database: envString("MONGO_DATABASE", "ngawimusical"),
	}
	if cfg.Mongo.ConnectTimeout, err = envDuration("MONGO_CONNECT_TIMEOUT", 5*time.Second); err != nil {
		errs = append(errs, err)
	}

	cfg.Auth = AuthConfig{
		Issuer:            envString("JWT_ISSUER", "ngawimusical"),
		Audience:          envString("JWT_AUDIENCE", "ngawimusical-api"),
		AccessSecret:      envString("JWT_ACCESS_SECRET", "development-access-secret-change-me"),
		AdmissionSecret:   envString("ADMISSION_PASS_SECRET", "development-admission-secret-change-me"),
		TicketTokenEncKey: envString("TICKET_TOKEN_ENC_KEY", "development-ticket-key-change-me"),
	}
	if cfg.Auth.AccessTTL, err = envDuration("JWT_ACCESS_TTL", 15*time.Minute); err != nil {
		errs = append(errs, err)
	}
	if cfg.Auth.RefreshTTL, err = envDuration("REFRESH_TOKEN_TTL", 7*24*time.Hour); err != nil {
		errs = append(errs, err)
	}
	if cfg.Auth.AdmissionTTL, err = envDuration("ADMISSION_PASS_TTL", time.Minute); err != nil {
		errs = append(errs, err)
	}
	if cfg.Auth.BcryptCost, err = envInt("BCRYPT_COST", 12); err != nil {
		errs = append(errs, err)
	}
	if cfg.Auth.LoginRateLimitPerMin, err = envInt("LOGIN_RATE_LIMIT_PER_MINUTE", 5); err != nil {
		errs = append(errs, err)
	}

	if cfg.Business.HoldTTL, err = envDuration("HOLD_TTL", 10*time.Minute); err != nil {
		errs = append(errs, err)
	}
	if cfg.Business.ReleaseGrace, err = envDuration("RELEASE_GRACE", 2*time.Minute); err != nil {
		errs = append(errs, err)
	}
	if cfg.Business.QueueEntryIdleTimeout, err = envDuration("QUEUE_ENTRY_IDLE_TIMEOUT", 5*time.Minute); err != nil {
		errs = append(errs, err)
	}
	if cfg.Business.QueueHeartbeatInterval, err = envDuration("QUEUE_HEARTBEAT_INTERVAL", 30*time.Second); err != nil {
		errs = append(errs, err)
	}
	if cfg.Business.MaxTicketsPerUser, err = envInt("MAX_TICKETS_PER_USER", 4); err != nil {
		errs = append(errs, err)
	}
	if cfg.Business.MaxSwapPerHold, err = envInt("MAX_SWAP_PER_HOLD", 2); err != nil {
		errs = append(errs, err)
	}
	if cfg.Business.AdmissionPassesPerSec, err = envInt("ADMISSION_PASSES_PER_SECOND", 50); err != nil {
		errs = append(errs, err)
	}
	if cfg.Business.ReserveRatePerUser, err = envInt("RESERVE_RATE_PER_USER", 2); err != nil {
		errs = append(errs, err)
	}
	if cfg.Business.ReserveBurstPerUser, err = envInt("RESERVE_BURST_PER_USER", 2); err != nil {
		errs = append(errs, err)
	}

	if cfg.Worker.OutboxRelayInterval, err = envDuration("OUTBOX_RELAY_INTERVAL", time.Second); err != nil {
		errs = append(errs, err)
	}
	if cfg.Worker.OutboxLease, err = envDuration("OUTBOX_LEASE", 30*time.Second); err != nil {
		errs = append(errs, err)
	}
	if cfg.Worker.ExpiryScanInterval, err = envDuration("EXPIRY_SCAN_INTERVAL", 5*time.Second); err != nil {
		errs = append(errs, err)
	}
	if cfg.Worker.PaymentReconcileEvery, err = envDuration("PAYMENT_RECONCILE_INTERVAL", 15*time.Second); err != nil {
		errs = append(errs, err)
	}

	cfg.Observability = ObservabilityConfig{
		OTLPEndpoint: envString("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
	}
	if cfg.Observability.MetricsEnabled, err = envBool("METRICS_ENABLED", true); err != nil {
		errs = append(errs, err)
	}
	if cfg.Observability.MetricsPort, err = envInt("METRICS_PORT", 9090); err != nil {
		errs = append(errs, err)
	}

	if cfg.App.Port < 1 || cfg.App.Port > 65535 {
		errs = append(errs, fmt.Errorf("APP_PORT harus antara 1 dan 65535"))
	}
	if cfg.DB.Name == "" || cfg.DB.User == "" {
		errs = append(errs, fmt.Errorf("DB_NAME dan DB_USER wajib diisi"))
	}
	if cfg.DB.MaxConns < 1 {
		errs = append(errs, fmt.Errorf("DB_MAX_CONNS harus lebih dari 0"))
	}
	if cfg.DB.MinConns < 0 {
		errs = append(errs, fmt.Errorf("DB_MIN_CONNS tidak boleh negatif"))
	}
	if cfg.Business.HoldTTL <= 0 {
		errs = append(errs, fmt.Errorf("HOLD_TTL harus lebih dari 0"))
	}
	if cfg.Business.ReleaseGrace <= 0 {
		errs = append(errs, fmt.Errorf("RELEASE_GRACE harus lebih dari 0"))
	}
	if cfg.Worker.OutboxRelayInterval <= 0 {
		errs = append(errs, fmt.Errorf("OUTBOX_RELAY_INTERVAL harus lebih dari 0"))
	}
	if cfg.Worker.ExpiryScanInterval <= 0 {
		errs = append(errs, fmt.Errorf("EXPIRY_SCAN_INTERVAL harus lebih dari 0"))
	}
	if cfg.Worker.PaymentReconcileEvery <= 0 {
		errs = append(errs, fmt.Errorf("PAYMENT_RECONCILE_INTERVAL harus lebih dari 0"))
	}
	if len(errs) > 0 {
		return Config{}, fmt.Errorf("konfigurasi tidak valid: %w", joinErrors(errs))
	}

	return cfg, nil
}

// DSN menghasilkan connection string PostgreSQL untuk pgx.
func (c DBConfig) DSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   net.JoinHostPort(c.Host, c.Port),
		Path:   c.Name,
	}
	query := u.Query()
	query.Set("sslmode", c.SSLMode)
	u.RawQuery = query.Encode()
	return u.String()
}

func envString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	raw := envString(key, "")
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s harus berupa angka: %w", key, err)
	}
	return value, nil
}

func envBool(key string, fallback bool) (bool, error) {
	raw := envString(key, "")
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s harus bernilai true atau false: %w", key, err)
	}
	return value, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := envString(key, "")
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s harus berupa durasi, contoh 10m atau 30s: %w", key, err)
	}
	return value, nil
}

func joinErrors(errs []error) error {
	if len(errs) == 1 {
		return errs[0]
	}
	message := ""
	for i, err := range errs {
		if i > 0 {
			message += "; "
		}
		message += err.Error()
	}
	return fmt.Errorf("%s", message)
}
