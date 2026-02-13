package dht

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-record"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/routing"
	ma "github.com/multiformats/go-multiaddr"
)

// NetworkConfig captures runtime options for the libp2p-backed DHT service.
type NetworkConfig struct {
	Namespace      string
	ListenAddrs    []string
	BootstrapPeers []string
	ProtocolID     string
	MaxRecordBytes int
	RequestTimeout time.Duration
}

// DefaultNetworkConfig returns conservative defaults suitable for local development.
func DefaultNetworkConfig() NetworkConfig {
	return NetworkConfig{
		Namespace:      "contentgrid",
		ListenAddrs:    []string{"/ip4/0.0.0.0/tcp/0"},
		ProtocolID:     "/contentgrid/kad/1.0.0",
		MaxRecordBytes: 64 * 1024,
		RequestTimeout: 10 * time.Second,
	}
}

// Validate ensures the configuration contains sensible values.
func (c NetworkConfig) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("namespace required")
	}
	if len(c.ListenAddrs) == 0 {
		return fmt.Errorf("at least one listen address required")
	}
	if c.ProtocolID == "" {
		return fmt.Errorf("protocol id required")
	}
	if c.MaxRecordBytes <= 0 {
		return fmt.Errorf("max record bytes must be positive")
	}
	if c.RequestTimeout <= 0 {
		return fmt.Errorf("request timeout must be positive")
	}
	for _, addr := range c.ListenAddrs {
		if _, err := ma.NewMultiaddr(addr); err != nil {
			return fmt.Errorf("invalid listen multiaddr %q: %w", addr, err)
		}
	}
	for _, addr := range c.BootstrapPeers {
		if _, err := ma.NewMultiaddr(addr); err != nil {
			return fmt.Errorf("invalid bootstrap multiaddr %q: %w", addr, err)
		}
	}
	return nil
}

// NetworkService implements a network-backed DHT node using libp2p Kademlia.
type NetworkService struct {
	host   host.Host
	dht    *dht.IpfsDHT
	cfg    NetworkConfig
	cancel context.CancelFunc
}

type networkEnvelope struct {
	UpdatedAt int64    `json:"updated_at"`
	Records   []Record `json:"records"`
}

type networkRecordValidator struct {
	maxBytes int
}

func (v networkRecordValidator) Validate(_ string, value []byte) error {
	if len(value) == 0 {
		return errors.New("record payload required")
	}
	if len(value) > v.maxBytes {
		return fmt.Errorf("record payload exceeds max size: %d > %d", len(value), v.maxBytes)
	}
	var env networkEnvelope
	if err := json.Unmarshal(value, &env); err != nil {
		return fmt.Errorf("invalid record encoding: %w", err)
	}
	if env.UpdatedAt <= 0 {
		return errors.New("record updated_at must be positive")
	}
	if len(env.Records) == 0 {
		return errors.New("record list must contain at least one entry")
	}
	for _, rec := range env.Records {
		if err := rec.Validate(); err != nil {
			return fmt.Errorf("invalid record: %w", err)
		}
	}
	return nil
}

func (v networkRecordValidator) Select(_ string, values [][]byte) (int, error) {
	if len(values) == 0 {
		return -1, errors.New("no values provided")
	}
	bestIdx := 0
	bestStamp := int64(0)
	bestSize := len(values[0])
	for i, raw := range values {
		var env networkEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return -1, fmt.Errorf("invalid record during select: %w", err)
		}
		if env.UpdatedAt > bestStamp || (env.UpdatedAt == bestStamp && len(raw) < bestSize) {
			bestIdx = i
			bestStamp = env.UpdatedAt
			bestSize = len(raw)
		}
	}
	return bestIdx, nil
}

// NewNetworkService constructs a libp2p-backed DHT node.
func NewNetworkService(ctx context.Context, cfg NetworkConfig) (*NetworkService, error) {
	if cfg.Namespace == "" {
		defaults := DefaultNetworkConfig()
		cfg.Namespace = defaults.Namespace
	}
	if len(cfg.ListenAddrs) == 0 {
		cfg.ListenAddrs = DefaultNetworkConfig().ListenAddrs
	}
	if cfg.ProtocolID == "" {
		cfg.ProtocolID = DefaultNetworkConfig().ProtocolID
	}
	if cfg.MaxRecordBytes == 0 {
		cfg.MaxRecordBytes = DefaultNetworkConfig().MaxRecordBytes
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = DefaultNetworkConfig().RequestTimeout
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	host, err := libp2p.New(libp2p.ListenAddrStrings(cfg.ListenAddrs...))
	if err != nil {
		return nil, fmt.Errorf("create libp2p host: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)

	validator := record.NamespacedValidator{
		cfg.Namespace: networkRecordValidator{maxBytes: cfg.MaxRecordBytes},
	}

	kdht, err := dht.New(ctx, host,
		dht.Mode(dht.ModeServer),
		dht.Validator(validator),
		dht.ProtocolPrefix(protocol.ID(cfg.ProtocolID)),
	)
	if err != nil {
		cancel()
		host.Close()
		return nil, fmt.Errorf("create dht: %w", err)
	}

	if err := kdht.Bootstrap(ctx); err != nil {
		kdht.Close()
		cancel()
		host.Close()
		return nil, fmt.Errorf("bootstrap dht: %w", err)
	}

	svc := &NetworkService{
		host:   host,
		dht:    kdht,
		cfg:    cfg,
		cancel: cancel,
	}

	if len(cfg.BootstrapPeers) > 0 {
		if err := svc.connectToPeers(cfg.BootstrapPeers); err != nil {
			svc.Close()
			return nil, err
		}
	}

	return svc, nil
}

// ID returns the peer identifier for the node.
func (s *NetworkService) ID() string {
	return s.host.ID().String()
}

// Addresses exposes the multiaddresses (with peer ID) that peers can use for bootstrapping.
func (s *NetworkService) Addresses() []string {
	info := peer.AddrInfo{ID: s.host.ID(), Addrs: s.host.Addrs()}
	addrs, err := peer.AddrInfoToP2pAddrs(&info)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		out = append(out, addr.String())
	}
	return out
}

// Close gracefully shuts down the DHT node.
func (s *NetworkService) Close() error {
	s.cancel()
	if err := s.dht.Close(); err != nil {
		s.host.Close()
		return fmt.Errorf("close dht: %w", err)
	}
	if err := s.host.Close(); err != nil {
		return fmt.Errorf("close host: %w", err)
	}
	return nil
}

// Publish stores or updates a record for the provided key.
func (s *NetworkService) Publish(key string, record Record) error {
	if key == "" {
		return fmt.Errorf("key required")
	}
	if err := record.Validate(); err != nil {
		return err
	}

	nsKey := s.namespacedKey(key)

	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.RequestTimeout)
	defer cancel()

	env, err := s.fetchEnvelope(ctx, nsKey)
	if err != nil {
		return fmt.Errorf("fetch envelope: %w", err)
	}

	updated := false
	for i, rec := range env.Records {
		if rec.ID == record.ID {
			env.Records[i] = cloneRecord(record)
			updated = true
			break
		}
	}
	if !updated {
		env.Records = append(env.Records, cloneRecord(record))
	}
	env.UpdatedAt = time.Now().UnixNano()

	payload, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("encode record: %w", err)
	}
	if len(payload) > s.cfg.MaxRecordBytes {
		return fmt.Errorf("record payload exceeds max size: %d > %d", len(payload), s.cfg.MaxRecordBytes)
	}

	deadline := time.Now().Add(s.cfg.RequestTimeout)
	var lastErr error
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			if lastErr != nil {
				return fmt.Errorf("put value: %w", lastErr)
			}
			return fmt.Errorf("put value: timeout after %s", s.cfg.RequestTimeout)
		}
		putCtx, putCancel := context.WithTimeout(context.Background(), remaining)
		err = s.dht.PutValue(putCtx, nsKey, payload)
		putCancel()
		if err == nil {
			return nil
		}
		if !retryablePutError(err) {
			return fmt.Errorf("put value: %w", err)
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}
}

// Find retrieves records for the given key.
func (s *NetworkService) Find(key string, limit int) ([]Record, error) {
	if key == "" {
		return nil, fmt.Errorf("key required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.RequestTimeout)
	defer cancel()

	env, err := s.fetchEnvelope(ctx, s.namespacedKey(key))
	if err != nil {
		if errors.Is(err, routing.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if len(env.Records) == 0 {
		return nil, nil
	}

	out := make([]Record, 0, len(env.Records))
	count := len(env.Records)
	if limit > 0 && limit < count {
		count = limit
	}
	for i := 0; i < count; i++ {
		out = append(out, cloneRecord(env.Records[i]))
	}
	return out, nil
}

func (s *NetworkService) namespacedKey(key string) string {
	return fmt.Sprintf("/%s/%s", s.cfg.Namespace, key)
}

func (s *NetworkService) fetchEnvelope(ctx context.Context, key string) (networkEnvelope, error) {
	raw, err := s.dht.GetValue(ctx, key)
	if err != nil {
		if errors.Is(err, routing.ErrNotFound) || errors.Is(err, dht.ErrNoPeersQueried) {
			return networkEnvelope{Records: make([]Record, 0)}, nil
		}
		return networkEnvelope{}, err
	}
	var env networkEnvelope
	if len(raw) == 0 {
		env.Records = make([]Record, 0)
		return env, nil
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return networkEnvelope{}, fmt.Errorf("decode envelope: %w", err)
	}
	if env.Records == nil {
		env.Records = make([]Record, 0)
	}
	return env, nil
}

func (s *NetworkService) connectToPeers(addresses []string) error {
	for _, addr := range addresses {
		maAddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			return fmt.Errorf("parse bootstrap %q: %w", addr, err)
		}
		info, err := peer.AddrInfoFromP2pAddr(maAddr)
		if err != nil {
			return fmt.Errorf("bootstrap address %q missing peer id: %w", addr, err)
		}
		if info.ID == s.host.ID() {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), s.cfg.RequestTimeout)
		err = s.host.Connect(ctx, *info)
		cancel()
		if err != nil {
			return fmt.Errorf("connect to bootstrap %s: %w", addr, err)
		}
	}
	return nil
}

func retryablePutError(err error) bool {
	if errors.Is(err, dht.ErrNoPeersQueried) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "failed to find any peer in table") || strings.Contains(msg, "failed to query any peers")
}
