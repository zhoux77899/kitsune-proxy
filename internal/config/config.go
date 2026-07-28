// Package config owns the on-disk configuration contract and validation.
package config

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	DefaultPort     = 18080
	DefaultLogLevel = "info"
	ConfigVersion   = 1
)

var upstreamNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// Paths contains all fixed user-owned paths used by Kitsune Proxy.
type Paths struct {
	BaseDir    string
	ConfigFile string
	LogsDir    string
}

// DefaultPaths returns the fixed ~/.kitsune paths.
func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve user home: %w", err)
	}

	base := filepath.Join(home, ".kitsune")
	return Paths{
		BaseDir:    base,
		ConfigFile: filepath.Join(base, "config.yaml"),
		LogsDir:    filepath.Join(base, "logs"),
	}, nil
}

// Config is the versioned user configuration.
type Config struct {
	Version   int                       `yaml:"version"`
	Server    ServerConfig              `yaml:"server"`
	Logging   LoggingConfig             `yaml:"logging"`
	Upstreams map[string]UpstreamConfig `yaml:"upstreams"`
}

// ServerConfig configures the fixed-loopback listener and local authentication.
type ServerConfig struct {
	Port   int    `yaml:"port"`
	APIKey string `yaml:"api_key"`
}

// LoggingConfig configures runtime log filtering.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// UpstreamConfig describes one upstream base URL, authentication rule, and models.
type UpstreamConfig struct {
	URL    string        `yaml:"url"`
	TLS    TLSConfig     `yaml:"tls,omitempty"`
	Auth   AuthConfig    `yaml:"auth"`
	Models []ModelConfig `yaml:"models"`
}

// TLSConfig controls TLS verification for one upstream.
type TLSConfig struct {
	SkipVerify bool `yaml:"skip_verify"`
}

// AuthConfig discriminates between API-key replacement and no upstream auth.
type AuthConfig struct {
	Mode   string  `yaml:"mode"`
	APIKey *string `yaml:"api_key,omitempty"`
}

// ModelConfig maps an optional public alias to an upstream model ID.
type ModelConfig struct {
	ID    string `yaml:"id"`
	Alias string `yaml:"alias,omitempty"`
}

// PublicName returns the model name accepted and advertised by Kitsune Proxy.
func (m ModelConfig) PublicName() string {
	if m.Alias != "" {
		return m.Alias
	}
	return m.ID
}

// ValidationError identifies a safe configuration field without exposing values.
type ValidationError struct {
	Path   string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("config: %s: %s", e.Path, e.Reason)
}

// Ensure creates the fixed user directory and a safe first-run template.
func Ensure(paths Paths) (bool, error) {
	if err := os.MkdirAll(paths.BaseDir, 0o700); err != nil {
		return false, fmt.Errorf("create config directory: %w", err)
	}
	_ = os.Chmod(paths.BaseDir, 0o700)

	_, err := os.Stat(paths.ConfigFile)
	switch {
	case err == nil:
		if err := os.Chmod(paths.ConfigFile, 0o600); err != nil {
			return false, fmt.Errorf("secure config file: %w", err)
		}
		return false, nil
	case !errors.Is(err, os.ErrNotExist):
		return false, fmt.Errorf("inspect config file: %w", err)
	}

	apiKey, err := generateLocalAPIKey()
	if err != nil {
		return false, fmt.Errorf("generate local API key: %w", err)
	}

	template := fmt.Sprintf(`version: 1

server:
  port: %d
  api_key: %s

logging:
  level: info

upstreams: {}

# Example:
# upstreams:
#   openrouter:
#     url: https://openrouter.ai/api
#     auth:
#       mode: replace
#       api_key: sk-upstream-key
#     models:
#       - id: nvidia/nemotron-3-ultra-550b-a55b:free
#         alias: openrouter-nemotron
#
#   ollama:
#     url: http://127.0.0.1:11434
#     auth:
#       mode: none
#     models:
#       - id: llama3.3
#
# For a trusted internal HTTPS service whose certificate cannot be verified:
#   internal-service:
#     url: https://10.0.0.1:8443/api
#     # WARNING: disables certificate chain, SAN, and hostname verification.
#     tls:
#       skip_verify: true
#     auth:
#       mode: none
#     models:
#       - id: internal-model
`, DefaultPort, apiKey)

	file, err := os.OpenFile(paths.ConfigFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create config file: %w", err)
	}

	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = os.Remove(paths.ConfigFile)
		}
	}()

	if _, err := io.WriteString(file, template); err != nil {
		return false, fmt.Errorf("write config file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return false, fmt.Errorf("sync config file: %w", err)
	}
	if err := file.Close(); err != nil {
		return false, fmt.Errorf("close config file: %w", err)
	}
	_ = os.Chmod(paths.ConfigFile, 0o600)
	complete = true
	return true, nil
}

// Load strictly decodes and validates a configuration file.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config: %w", err)
	}

	nodeDecoder := yaml.NewDecoder(bytes.NewReader(data))
	nodeDecoder.KnownFields(true)
	var document yaml.Node
	if err := nodeDecoder.Decode(&document); err != nil {
		return Config{}, &ValidationError{Path: "document", Reason: "invalid YAML structure"}
	}
	var extra yaml.Node
	if err := nodeDecoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Config{}, &ValidationError{Path: "document", Reason: "multiple YAML documents are not supported"}
		}
		return Config{}, &ValidationError{Path: "document", Reason: "invalid trailing YAML document"}
	}
	if len(document.Content) != 1 {
		return Config{}, &ValidationError{Path: "document", Reason: "must contain one mapping"}
	}
	if err := validateDocumentSchema(document.Content[0]); err != nil {
		return Config{}, err
	}

	cfg := Config{
		Server:  ServerConfig{Port: DefaultPort},
		Logging: LoggingConfig{Level: DefaultLogLevel},
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, &ValidationError{Path: "document", Reason: "invalid YAML structure"}
	}

	if cfg.Upstreams == nil {
		cfg.Upstreams = make(map[string]UpstreamConfig)
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

type schemaField func(*yaml.Node, string) error

func validateDocumentSchema(node *yaml.Node) error {
	return validateMapping(node, "", map[string]schemaField{
		"version":   scalarSchema("!!int"),
		"server":    validateServerSchema,
		"logging":   validateLoggingSchema,
		"upstreams": validateUpstreamsSchema,
	})
}

func validateServerSchema(node *yaml.Node, path string) error {
	return validateMapping(node, path, map[string]schemaField{
		"port":    scalarSchema("!!int"),
		"api_key": scalarSchema("!!str"),
	})
}

func validateLoggingSchema(node *yaml.Node, path string) error {
	return validateMapping(node, path, map[string]schemaField{
		"level": scalarSchema("!!str"),
	})
}

func validateUpstreamsSchema(node *yaml.Node, path string) error {
	if node.Kind != yaml.MappingNode {
		return validation(path, "must be a mapping")
	}
	seen := make(map[string]struct{})
	for index := 0; index < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return validation(path, "names must be strings")
		}
		if _, exists := seen[key.Value]; exists {
			return validation(path+"."+key.Value, "duplicate key")
		}
		seen[key.Value] = struct{}{}
		if err := validateUpstreamSchema(value, path+"."+key.Value); err != nil {
			return err
		}
	}
	return nil
}

func validateUpstreamSchema(node *yaml.Node, path string) error {
	return validateMapping(node, path, map[string]schemaField{
		"url":    scalarSchema("!!str"),
		"tls":    validateTLSSchema,
		"auth":   validateAuthSchema,
		"models": validateModelsSchema,
	})
}

func validateTLSSchema(node *yaml.Node, path string) error {
	return validateMapping(node, path, map[string]schemaField{
		"skip_verify": scalarSchema("!!bool"),
	})
}

func validateAuthSchema(node *yaml.Node, path string) error {
	return validateMapping(node, path, map[string]schemaField{
		"mode":    scalarSchema("!!str"),
		"api_key": scalarSchema("!!str"),
	})
}

func validateModelsSchema(node *yaml.Node, path string) error {
	if node.Kind != yaml.SequenceNode {
		return validation(path, "must be a sequence")
	}
	for index, model := range node.Content {
		modelPath := fmt.Sprintf("%s[%d]", path, index)
		if err := validateMapping(model, modelPath, map[string]schemaField{
			"id":    scalarSchema("!!str"),
			"alias": scalarSchema("!!str"),
		}); err != nil {
			return err
		}
	}
	return nil
}

func validateMapping(node *yaml.Node, path string, fields map[string]schemaField) error {
	displayPath := path
	if displayPath == "" {
		displayPath = "document"
	}
	if node.Kind != yaml.MappingNode {
		return validation(displayPath, "must be a mapping")
	}

	seen := make(map[string]struct{})
	for index := 0; index < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return validation(displayPath, "field names must be strings")
		}
		fieldPath := key.Value
		if path != "" {
			fieldPath = path + "." + key.Value
		}
		if _, exists := seen[key.Value]; exists {
			return validation(fieldPath, "duplicate key")
		}
		seen[key.Value] = struct{}{}
		validateField, ok := fields[key.Value]
		if !ok {
			return validation(fieldPath, "unknown field")
		}
		if err := validateField(value, fieldPath); err != nil {
			return err
		}
	}
	return nil
}

func scalarSchema(tag string) schemaField {
	return func(node *yaml.Node, path string) error {
		if node.Kind != yaml.ScalarNode || node.Tag != tag {
			return validation(path, "has an invalid type")
		}
		return nil
	}
}

// Validate enforces the complete public configuration contract.
func Validate(cfg Config) error {
	if cfg.Version != ConfigVersion {
		return validation("version", "must be 1")
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return validation("server.port", "must be between 1 and 65535")
	}
	if err := validateSecret(cfg.Server.APIKey); err != nil {
		return validation("server.api_key", err.Error())
	}

	switch cfg.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return validation("logging.level", "must be debug, info, warn, or error")
	}

	publicModels := make(map[string]string)
	for name, upstream := range cfg.Upstreams {
		basePath := "upstreams." + name
		if !upstreamNamePattern.MatchString(name) {
			return validation(basePath, "name must contain 1-64 letters, digits, dots, underscores, or hyphens")
		}
		if err := validateBaseURL(upstream.URL); err != nil {
			return validation(basePath+".url", err.Error())
		}
		if upstream.TLS.SkipVerify {
			parsed, _ := url.Parse(upstream.URL)
			if parsed.Scheme != "https" {
				return validation(basePath+".tls.skip_verify", "requires an HTTPS upstream URL")
			}
		}
		if err := validateAuth(basePath+".auth", upstream.Auth); err != nil {
			return err
		}
		if len(upstream.Models) == 0 {
			return validation(basePath+".models", "must contain at least one model")
		}

		for index, model := range upstream.Models {
			modelPath := fmt.Sprintf("%s.models[%d]", basePath, index)
			if err := validateModelName(model.ID); err != nil {
				return validation(modelPath+".id", err.Error())
			}
			if model.Alias != "" {
				if err := validateModelName(model.Alias); err != nil {
					return validation(modelPath+".alias", err.Error())
				}
			}

			publicName := model.PublicName()
			if previous, exists := publicModels[publicName]; exists {
				return validation(modelPath, fmt.Sprintf("public model name conflicts with upstream %s", previous))
			}
			publicModels[publicName] = name
		}
	}
	return nil
}

func validation(path, reason string) error {
	return &ValidationError{Path: path, Reason: reason}
}

func validateBaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("must be an absolute HTTP or HTTPS base URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("scheme must be http or https")
	}
	if parsed.User != nil {
		return errors.New("userinfo is not allowed")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return errors.New("query is not allowed")
	}
	if parsed.Fragment != "" || strings.Contains(raw, "#") {
		return errors.New("fragment is not allowed")
	}
	return nil
}

func validateAuth(path string, auth AuthConfig) error {
	switch auth.Mode {
	case "replace":
		if auth.APIKey == nil {
			return validation(path+".api_key", "is required when mode is replace")
		}
		if err := validateSecret(*auth.APIKey); err != nil {
			return validation(path+".api_key", err.Error())
		}
	case "none":
		if auth.APIKey != nil {
			return validation(path+".api_key", "must be omitted when mode is none")
		}
	default:
		return validation(path+".mode", "must be replace or none")
	}
	return nil
}

func validateSecret(secret string) error {
	if secret == "" {
		return errors.New("must not be empty")
	}
	if len(secret) > 4096 {
		return errors.New("must not exceed 4096 bytes")
	}
	if strings.TrimSpace(secret) != secret {
		return errors.New("must not contain leading or trailing whitespace")
	}
	if strings.ContainsAny(secret, "\r\n") {
		return errors.New("must not contain line breaks")
	}
	return nil
}

func validateModelName(name string) error {
	if name == "" {
		return errors.New("must not be empty")
	}
	if len(name) > 256 {
		return errors.New("must not exceed 256 bytes")
	}
	if strings.TrimSpace(name) != name {
		return errors.New("must not contain leading or trailing whitespace")
	}
	if strings.ContainsAny(name, "\r\n\x00") {
		return errors.New("must not contain control line breaks")
	}
	return nil
}

func generateLocalAPIKey() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return "kitsune-" + base64.RawURLEncoding.EncodeToString(random), nil
}
