package core

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/ipfs/go-cid"
	ipld "github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/multiformats/go-multihash"

	"github.com/qall-project/qall-daemon/server/registry"
)

type mockRegistry struct {
	registry.RegistryProvider
	blocks map[string][]byte
}

func (m *mockRegistry) GetBlock(ctx context.Context, hash string) ([]byte, error) {
	data, exists := m.blocks[hash]
	if !exists {
		return nil, fmt.Errorf("block not found in mock registry")
	}
	return data, nil
}

func createNodeBlock(t *testing.T, payloadHash string) (string, []byte) {
	nb := basicnode.Prototype.Map.NewBuilder()
	ma, _ := nb.BeginMap(2)

	ma.AssembleKey().AssignString("type")
	ma.AssembleValue().AssignString("node")

	ma.AssembleKey().AssignString("payload_hash")
	ma.AssembleValue().AssignString(payloadHash)

	ma.Finish()
	return encodeAndHash(t, nb.Build())
}

func createUnknownBlock(t *testing.T) (string, []byte) {
	nb := basicnode.Prototype.Map.NewBuilder()
	ma, _ := nb.BeginMap(2)

	ma.AssembleKey().AssignString("type")
	ma.AssembleValue().AssignString("unknown")

	ma.Finish()
	return encodeAndHash(t, nb.Build())
}

func createPayloadBlock(t *testing.T, code string, env map[string]string) (string, []byte) {
	nb := basicnode.Prototype.Map.NewBuilder()
	ma, _ := nb.BeginMap(3)

	ma.AssembleKey().AssignString("type")
	ma.AssembleValue().AssignString("payload")

	ma.AssembleKey().AssignString("code")
	ma.AssembleValue().AssignString(code)

	ma.AssembleKey().AssignString("environment")
	envMa, _ := ma.AssembleValue().BeginMap(int64(len(env)))
	for k, v := range env {
		envMa.AssembleKey().AssignString(k)
		envMa.AssembleValue().AssignString(v)
	}
	envMa.Finish()

	ma.Finish()
	return encodeAndHash(t, nb.Build())
}

func encodeAndHash(t *testing.T, node ipld.Node) (string, []byte) {
	var buf bytes.Buffer
	if err := dagcbor.Encode(node, &buf); err != nil {
		t.Fatalf("Failed to encode DAG-CBOR: %v", err)
	}
	data := buf.Bytes()

	pref := cid.Prefix{
		Version:  1,
		Codec:    cid.DagCBOR,
		MhType:   multihash.SHA2_256,
		MhLength: -1,
	}
	c, err := pref.Sum(data)
	if err != nil {
		t.Fatalf("Failed to compute CID: %v", err)
	}
	return c.String(), data
}

func TestExtractNode_ValidNodeResolution(t *testing.T) {
	payloadHash, payloadData := createPayloadBlock(t, "def task(): pass", map[string]string{
		"image": "python:3.11",
	})
	nodeHash, nodeData := createNodeBlock(t, payloadHash)

	reg := &mockRegistry{
		blocks: map[string][]byte{
			payloadHash: payloadData,
			nodeHash:    nodeData,
		},
	}

	payload, err := extractNode(context.Background(), nodeHash, reg)

	if err != nil {
		t.Fatalf("Expected successful traversal, got: %v", err)
	}

	if payload.Type != "payload" {
		t.Errorf("Expected type 'payload', got '%s'", payload.Type)
	}
	if payload.Code != "def task(): pass" {
		t.Errorf("Failed to resolve the underlying payload")
	}
	if payload.Environment["image"] != "python:3.11" {
		t.Errorf("Environment mismatch")
	}
}

func TestExtractNode_PayloadBlock(t *testing.T) {
	payloadHash, payloadData := createPayloadBlock(t, "print('Hello Qall')", map[string]string{
		"image": "python:3.11",
	})

	reg := &mockRegistry{
		blocks: map[string][]byte{
			payloadHash: payloadData,
		},
	}

	_, err := extractNode(context.Background(), payloadHash, reg)
	if err == nil {
		t.Fatalf("Expected an error \"encountered a payload block xxx without a parent node block\", got none")
	}
}

func TestExtractNode_UnknownBlock(t *testing.T) {
	unknownHash, unknownData := createUnknownBlock(t)

	reg := &mockRegistry{
		blocks: map[string][]byte{
			unknownHash: unknownData,
		},
	}

	_, err := extractNode(context.Background(), unknownHash, reg)
	if err == nil {
		t.Fatalf("Expected an error \"unknown or unsupported IPLD block type: unknown\", got none")
	}
}

func TestExtractPayload_ValidPayloadResolution(t *testing.T) {
	payloadHash, payloadData := createPayloadBlock(t, "def task(): pass", map[string]string{
		"image": "python:3.11",
	})

	reg := &mockRegistry{
		blocks: map[string][]byte{
			payloadHash: payloadData,
		},
	}

	payload, err := extractPayload(context.Background(), payloadHash, reg)

	if err != nil {
		t.Fatalf("Expected successful traversal, got: %v", err)
	}

	if payload.Type != "payload" {
		t.Errorf("Expected type 'payload', got '%s'", payload.Type)
	}
	if payload.Code != "def task(): pass" {
		t.Errorf("Failed to resolve the underlying payload")
	}
	if payload.Environment["image"] != "python:3.11" {
		t.Errorf("Environment mismatch")
	}
}

func TestExtractPayload_UnknownBlock(t *testing.T) {
	unknownHash, unknownData := createUnknownBlock(t)

	reg := &mockRegistry{
		blocks: map[string][]byte{
			unknownHash: unknownData,
		},
	}

	_, err := extractPayload(context.Background(), unknownHash, reg)
	if err == nil {
		t.Fatalf("Expected an error \"unknown or unsupported IPLD block type: unknown\", got none")
	}
}

func TestExtractPayload_MissingCodeField(t *testing.T) {
	nb := basicnode.Prototype.Map.NewBuilder()
	ma, _ := nb.BeginMap(1)
	ma.AssembleKey().AssignString("type")
	ma.AssembleValue().AssignString("payload")
	ma.Finish()

	blockHash, blockData := encodeAndHash(t, nb.Build())

	reg := &mockRegistry{
		blocks: map[string][]byte{
			blockHash: blockData,
		},
	}

	_, err := extractPayload(context.Background(), blockHash, reg)
	if err == nil || err.Error() != "mandatory 'code' field missing" {
		t.Fatalf("Expected mandatory 'code' error, got: %v", err)
	}
}

func TestVerifyCID_IntegrityFailure(t *testing.T) {
	validHash, validData := createPayloadBlock(t, "valid code", nil)

	corruptedData := append(validData, []byte("hack")...)

	err := verifyCID(corruptedData, validHash)
	if err == nil {
		t.Fatal("Expected verifyCID to fail due to cryptographic integrity violation")
	}
}
