package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

type ID string

func NewID(prefix string) (ID, error) {
	if err := ID(prefix).Validate("id_prefix"); err != nil {
		return "", err
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return ID(prefix + "-" + hex.EncodeToString(bytes[:])), nil
}

func (id ID) Validate(label string) error {
	if strings.TrimSpace(string(id)) == "" {
		return fmt.Errorf("%s is required", label)
	}
	if len(id) > 200 {
		return fmt.Errorf("%s is too long", label)
	}
	return nil
}

type ActorType string

const (
	ActorHuman  ActorType = "human"
	ActorAgent  ActorType = "agent"
	ActorSystem ActorType = "system"
)

func (a ActorType) Validate() error {
	switch a {
	case ActorHuman, ActorAgent, ActorSystem:
		return nil
	default:
		return fmt.Errorf("invalid actor type %q", a)
	}
}
