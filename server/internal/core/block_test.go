package core

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ipfs/go-cid"
	ipld "github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/multiformats/go-multihash"

	"github.com/qall-project/qall-daemon/server/internal/registry"
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

func createTaskNodeBlock(t *testing.T, payloadHash string) (string, []byte) {
	nb := basicnode.Prototype.Map.NewBuilder()
	ma, _ := nb.BeginMap(2)

	ma.AssembleKey().AssignString("type")
	ma.AssembleValue().AssignString("task_node")

	ma.AssembleKey().AssignString("payload_hash")
	ma.AssembleValue().AssignString(payloadHash)

	ma.Finish()
	return encodeAndHash(t, nb.Build())
}

func createTaskPayloadBlock(
	t *testing.T,
	code, codeFormat, qallVersion, image string,
	requirements []string,
	inputFormats []string,
) (string, []byte) {
	nb := basicnode.Prototype.Map.NewBuilder()
	ma, _ := nb.BeginMap(6)

	ma.AssembleKey().AssignString("type")
	ma.AssembleValue().AssignString("task_payload")

	ma.AssembleKey().AssignString("code_format")
	ma.AssembleValue().AssignString(codeFormat)

	ma.AssembleKey().AssignString("code")
	ma.AssembleValue().AssignString(code)

	ma.AssembleKey().AssignString("runtime")
	runMa, _ := ma.AssembleValue().BeginMap(1)
	runMa.AssembleKey().AssignString("qall_version")
	runMa.AssembleValue().AssignString(qallVersion)
	runMa.Finish()

	ma.AssembleKey().AssignString("environment")
	envMa, _ := ma.AssembleValue().BeginMap(2)
	envMa.AssembleKey().AssignString("image")
	envMa.AssembleValue().AssignString(image)
	envMa.AssembleKey().AssignString("requirements")
	reqMa, _ := envMa.AssembleValue().BeginList(int64(len(requirements)))
	for _, r := range requirements {
		reqMa.AssembleValue().AssignString(r)
	}
	reqMa.Finish()
	envMa.Finish()

	ma.AssembleKey().AssignString("metadata")
	metaMa, _ := ma.AssembleValue().BeginMap(1)
	metaMa.AssembleKey().AssignString("quantum_runs")
	qrMa, _ := metaMa.AssembleValue().BeginList(int64(len(inputFormats)))
	for _, f := range inputFormats {
		itemMa, _ := qrMa.AssembleValue().BeginMap(1)
		itemMa.AssembleKey().AssignString("input_format")
		itemMa.AssembleValue().AssignString(f)
		itemMa.Finish()
	}
	qrMa.Finish()
	metaMa.Finish()

	ma.Finish()
	return encodeAndHash(t, nb.Build())
}

func createUnknownBlock(t *testing.T) (string, []byte) {
	nb := basicnode.Prototype.Map.NewBuilder()
	ma, _ := nb.BeginMap(1)

	ma.AssembleKey().AssignString("type")
	ma.AssembleValue().AssignString("unknown")

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

func TestGetTaskPayload_ValidNodeResolution(t *testing.T) {
	payloadCode := "def task(): pass"
	payloadCodeFormat := "python"
	payloadQallVersion := "0.1.0"
	payloadImage := "python:3.11"
	payloadRequirements := []string{"numpy", "pandas"}
	payloadInputFormats := []string{"qasm", "quirk"}

	payloadHash, payloadData := createTaskPayloadBlock(
		t,
		payloadCode,
		payloadCodeFormat,
		payloadQallVersion,
		payloadImage,
		payloadRequirements,
		payloadInputFormats,
	)
	nodeHash, nodeData := createTaskNodeBlock(t, payloadHash)

	reg := &mockRegistry{
		blocks: map[string][]byte{
			payloadHash: payloadData,
			nodeHash:    nodeData,
		},
	}

	payload, err := getTaskPayload(context.Background(), nodeHash, reg)

	if err != nil {
		t.Fatalf("Expected successful traversal, got: %v", err)
	}

	if payload.Type != "task_payload" {
		t.Errorf("Expected type 'task_payload', got '%s'", payload.Type)
	}
	if payload.Code != payloadCode {
		t.Errorf("Failed to resolve the underlying payload")
	}
	if payload.CodeFormat != payloadCodeFormat {
		t.Errorf("Expected code format '%s', got '%s'", payloadCodeFormat, payload.CodeFormat)
	}
	if payload.RuntimeQallVersion != payloadQallVersion {
		t.Errorf("Expected qall version '%s', got '%s'", payloadQallVersion, payload.RuntimeQallVersion)
	}
	if payload.Image != payloadImage {
		t.Errorf("Expected image '%s', got '%s'", payloadImage, payload.Image)
	}
	if !reflect.DeepEqual(payload.Requirements, payloadRequirements) {
		t.Errorf("Expected requirements %v, got %v", payloadRequirements, payload.Requirements)
	}
	if !reflect.DeepEqual(payload.QuantumRunInputFormats, payloadInputFormats) {
		t.Errorf("Expected input formats %v, got %v", payloadInputFormats, payload.QuantumRunInputFormats)
	}
}

func TestGetTaskPayload_DirectPayloadBlock(t *testing.T) {
	payloadHash, payloadData := createTaskPayloadBlock(
		t,
		"print('Hello Qall')",
		"python",
		"0.1.0",
		"python:3.11",
		nil,
		nil,
	)

	reg := &mockRegistry{
		blocks: map[string][]byte{
			payloadHash: payloadData,
		},
	}

	_, err := getTaskPayload(context.Background(), payloadHash, reg)
	if err == nil {
		t.Fatalf("Expected an error for a payload block without a parent node block, got none")
	}
	if !strings.Contains(err.Error(), "without a parent node block") {
		t.Fatalf("Expected 'without a parent node block' error, got: %v", err)
	}
}

func TestGetTaskPayload_UnknownBlock(t *testing.T) {
	unknownHash, unknownData := createUnknownBlock(t)

	reg := &mockRegistry{
		blocks: map[string][]byte{
			unknownHash: unknownData,
		},
	}

	_, err := getTaskPayload(context.Background(), unknownHash, reg)
	if err == nil {
		t.Fatalf("Expected an error \"unknown or unsupported IPLD block type: unknown\", got none")
	}
	if !strings.Contains(err.Error(), "unknown or unsupported IPLD block type") {
		t.Fatalf("Expected 'unknown or unsupported IPLD block type' error, got: %v", err)
	}
}

func TestExtractTaskPayload_ValidPayloadResolution(t *testing.T) {
	payloadHash, payloadData := createTaskPayloadBlock(
		t,
		"def task(): pass",
		"python",
		"0.1.0",
		"python:3.11",
		nil,
		nil,
	)

	reg := &mockRegistry{
		blocks: map[string][]byte{
			payloadHash: payloadData,
		},
	}

	payload, err := extractTaskPayload(context.Background(), payloadHash, reg)

	if err != nil {
		t.Fatalf("Expected successful traversal, got: %v", err)
	}

	if payload.Type != "task_payload" {
		t.Errorf("Expected type 'task_payload', got '%s'", payload.Type)
	}
	if payload.Code != "def task(): pass" {
		t.Errorf("Failed to resolve the underlying payload")
	}
	if payload.Image != "python:3.11" {
		t.Errorf("Environment mismatch")
	}
}

func TestExtractTaskPayload_UnknownBlock(t *testing.T) {
	unknownHash, unknownData := createUnknownBlock(t)

	reg := &mockRegistry{
		blocks: map[string][]byte{
			unknownHash: unknownData,
		},
	}

	_, err := extractTaskPayload(context.Background(), unknownHash, reg)
	if err == nil {
		t.Fatalf("Expected an error \"unknown or unsupported IPLD block type: unknown\", got none")
	}
	if !strings.Contains(err.Error(), "unknown or unsupported IPLD block type") {
		t.Fatalf("Expected 'unknown or unsupported IPLD block type' error, got: %v", err)
	}
}

func TestExtractTaskPayload_MissingCodeField(t *testing.T) {
	nb := basicnode.Prototype.Map.NewBuilder()
	ma, _ := nb.BeginMap(1)
	ma.AssembleKey().AssignString("type")
	ma.AssembleValue().AssignString("task_payload")
	ma.Finish()

	blockHash, blockData := encodeAndHash(t, nb.Build())

	reg := &mockRegistry{
		blocks: map[string][]byte{
			blockHash: blockData,
		},
	}

	_, err := extractTaskPayload(context.Background(), blockHash, reg)
	if err == nil || err.Error() != "mandatory 'code' field missing" {
		t.Fatalf("Expected mandatory 'code' error, got: %v", err)
	}
}

func TestExtractTaskPayload_MissingRuntimeField(t *testing.T) {
	nb := basicnode.Prototype.Map.NewBuilder()
	ma, _ := nb.BeginMap(2)
	ma.AssembleKey().AssignString("type")
	ma.AssembleValue().AssignString("task_payload")
	ma.AssembleKey().AssignString("code")
	ma.AssembleValue().AssignString("print('hello')")
	ma.Finish()

	blockHash, blockData := encodeAndHash(t, nb.Build())

	reg := &mockRegistry{
		blocks: map[string][]byte{
			blockHash: blockData,
		},
	}

	_, err := extractTaskPayload(context.Background(), blockHash, reg)
	if err == nil || !strings.Contains(err.Error(), "mandatory 'runtime' field missing") {
		t.Fatalf("Expected mandatory 'runtime' error, got: %v", err)
	}
}

func TestVerifyCID_IntegrityFailure(t *testing.T) {
	validHash, validData := createTaskPayloadBlock(
		t,
		"valid code",
		"python",
		"0.1.0",
		"python:3.11",
		nil,
		nil,
	)

	corruptedData := append(validData, []byte("hack")...)

	err := verifyCID(corruptedData, validHash)
	if err == nil {
		t.Fatal("Expected verifyCID to fail due to cryptographic integrity violation")
	}
}
