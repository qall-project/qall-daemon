package core

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/node/basicnode"

	"qall-daemon-server/internal/object"
	"qall-daemon-server/internal/registry"
)

func getTaskPayload(ctx context.Context, hash string, reg registry.RegistryProvider) (*object.TaskPayload, error) {
	block, err := extractBlock(ctx, hash, reg)

	if err != nil {
		return nil, err
	}

	blockType, err := block.LookupByString("type")

	if err != nil {
		return nil, fmt.Errorf("'type' field missing in block %s", hash)
	}

	blockTypeStr, _ := blockType.AsString()

	if blockTypeStr == "task_node" {
		log.Printf("'task_node' block identified. Extracting payload pointer.")

		payloadHash, err := block.LookupByString("payload_hash")

		if err != nil {
			return nil, fmt.Errorf("'payload_hash' missing in task_node %s", hash)
		}

		payloadHashStr, _ := payloadHash.AsString()

		return extractTaskPayload(ctx, payloadHashStr, reg)
	} else if blockTypeStr == "task_payload" {
		return nil, fmt.Errorf("invalid hash reference: encountered a task_payload block %s without a parent node block", hash)
	} else {
		return nil, fmt.Errorf("unknown or unsupported IPLD block type: %s", blockTypeStr)
	}
}

func extractTaskPayload(ctx context.Context, hash string, reg registry.RegistryProvider) (*object.TaskPayload, error) {
	block, err := extractBlock(ctx, hash, reg)

	if err != nil {
		return nil, err
	}

	blockType, err := block.LookupByString("type")

	if err != nil {
		return nil, fmt.Errorf("'type' field missing in block %s", hash)
	}

	blockTypeStr, _ := blockType.AsString()

	if blockTypeStr != "task_payload" {
		return nil, fmt.Errorf("unknown or unsupported IPLD block type: %s", blockTypeStr)
	}

	log.Printf("'task_payload' block reached. Extracting execution metadata.")

	payload := &object.TaskPayload{
		Requirements:           make([]string, 0),
		QuantumRunInputFormats: make([]string, 0),
	}

	if typeNode, err := block.LookupByString("type"); err == nil {
		payload.Type, _ = typeNode.AsString()
	}
	if formatNode, err := block.LookupByString("code_format"); err == nil {
		payload.CodeFormat, _ = formatNode.AsString()
	}
	if codeNode, err := block.LookupByString("code"); err == nil {
		payload.Code, _ = codeNode.AsString()
	} else {
		return nil, fmt.Errorf("mandatory 'code' field missing")
	}

	if metadataNode, err := block.LookupByString("metadata"); err == nil {
		quantumRunsNode, err := metadataNode.LookupByString("quantum_runs")

		if err == nil && quantumRunsNode.Kind() == ipld.Kind_List {
			seen := make(map[string]struct{})
			iterator := quantumRunsNode.ListIterator()

			for !iterator.Done() {
				_, quantumRunNode, err := iterator.Next()

				if err != nil || quantumRunNode.Kind() != ipld.Kind_Map {
					continue
				}

				inputFormatNode, err := quantumRunNode.LookupByString("input_format")

				if err != nil || inputFormatNode.Kind() == ipld.Kind_Null {
					continue
				}

				inputFormat, err := inputFormatNode.AsString()

				if err != nil {
					continue
				}

				if _, exists := seen[inputFormat]; exists {
					continue
				}

				seen[inputFormat] = struct{}{}
				payload.QuantumRunInputFormats = append(payload.QuantumRunInputFormats, inputFormat)
			}
		}
	}

	if envNode, err := block.LookupByString("environment"); err == nil && envNode.Kind() == ipld.Kind_Map {
		if imgNode, err := envNode.LookupByString("image"); err == nil && imgNode.Kind() == ipld.Kind_String {
			payload.Image, _ = imgNode.AsString()
		}

		if reqNode, err := envNode.LookupByString("requirements"); err == nil {
			switch reqNode.Kind() {
			case ipld.Kind_List:
				listItr := reqNode.ListIterator()
				for !listItr.Done() {
					_, itemNode, _ := listItr.Next()
					if itemStr, err := itemNode.AsString(); err == nil && strings.TrimSpace(itemStr) != "" {
						payload.Requirements = append(payload.Requirements, itemStr)
					}
				}
			case ipld.Kind_String:
				if reqStr, err := reqNode.AsString(); err == nil && strings.TrimSpace(reqStr) != "" {
					payload.Requirements = append(payload.Requirements, strings.Fields(reqStr)...)
				}
			}
		}
	}

	return payload, nil
}

func getWorkerPayload(ctx context.Context, hash string, reg registry.RegistryProvider) (*object.WorkerPayload, error) {
	block, err := extractBlock(ctx, hash, reg)

	if err != nil {
		return nil, err
	}

	blockType, err := block.LookupByString("type")

	if err != nil {
		return nil, fmt.Errorf("'type' field missing in block %s", hash)
	}

	blockTypeStr, _ := blockType.AsString()

	if blockTypeStr == "worker_node" {
		log.Printf("'worker_node' block identified. Extracting payload pointer.")

		payloadHash, err := block.LookupByString("payload_hash")

		if err != nil {
			return nil, fmt.Errorf("'payload_hash' missing in worker_node %s", hash)
		}

		payloadHashStr, _ := payloadHash.AsString()

		return extractWorkerPayload(ctx, payloadHashStr, reg)
	} else if blockTypeStr == "worker_payload" {
		return nil, fmt.Errorf("invalid hash reference: encountered a worker_payload block %s without a parent node block", hash)
	} else {
		return nil, fmt.Errorf("unknown or unsupported IPLD block type: %s", blockTypeStr)
	}
}

func extractWorkerPayload(ctx context.Context, hash string, reg registry.RegistryProvider) (*object.WorkerPayload, error) {
	block, err := extractBlock(ctx, hash, reg)

	if err != nil {
		return nil, err
	}

	blockType, err := block.LookupByString("type")

	if err != nil {
		return nil, fmt.Errorf("'type' field missing in block %s", hash)
	}

	blockTypeStr, _ := blockType.AsString()

	if blockTypeStr != "worker_payload" {
		return nil, fmt.Errorf("unknown or unsupported IPLD block type: %s", blockTypeStr)
	}

	log.Printf("'worker_payload' block reached. Extracting execution metadata.")
	payload := &object.WorkerPayload{
		Requirements: make([]string, 0),
	}

	if typeNode, err := block.LookupByString("type"); err == nil {
		payload.Type, _ = typeNode.AsString()
	}

	if formatNode, err := block.LookupByString("code_format"); err == nil {
		payload.CodeFormat, _ = formatNode.AsString()
	}

	if codeNode, err := block.LookupByString("code"); err == nil {
		payload.Code, _ = codeNode.AsString()
	} else {
		return nil, fmt.Errorf("mandatory 'code' field missing")
	}

	if envNode, err := block.LookupByString("environment"); err == nil && envNode.Kind() == ipld.Kind_Map {
		if imgNode, err := envNode.LookupByString("image"); err == nil && imgNode.Kind() == ipld.Kind_String {
			payload.Image, _ = imgNode.AsString()
		}

		if reqNode, err := envNode.LookupByString("requirements"); err == nil {
			log.Println("reqNode", reqNode)

			switch reqNode.Kind() {
			case ipld.Kind_List:
				listItr := reqNode.ListIterator()

				for !listItr.Done() {
					_, itemNode, _ := listItr.Next()
					if itemStr, err := itemNode.AsString(); err == nil && strings.TrimSpace(itemStr) != "" {
						payload.Requirements = append(payload.Requirements, itemStr)
					}
				}
			case ipld.Kind_String:
				if reqStr, err := reqNode.AsString(); err == nil && strings.TrimSpace(reqStr) != "" {
					payload.Requirements = append(payload.Requirements, strings.Fields(reqStr)...)
				}
			}
		}
	}

	return payload, nil
}

func extractBlock(ctx context.Context, hash string, reg registry.RegistryProvider) (ipld.Node, error) {
	data, err := reg.GetBlock(ctx, hash)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch block %s: %w", hash, err)
	}

	if err := verifyCID(data, hash); err != nil {
		return nil, fmt.Errorf("integrity compromised for block %s: %w", hash, err)
	}

	node, err := decodeDAGCBOR(data)

	if err != nil {
		return nil, fmt.Errorf("DAG-CBOR validation failed for block %s: %w", hash, err)
	}

	return node, nil
}

func decodeDAGCBOR(data []byte) (ipld.Node, error) {
	nodeBuilder := basicnode.Prototype.Any.NewBuilder()
	dataReader := bytes.NewReader(data)

	if err := dagcbor.Decode(nodeBuilder, dataReader); err != nil {
		return nil, err
	}

	return nodeBuilder.Build(), nil
}

func verifyCID(data []byte, hash string) error {
	targetCID, err := cid.Decode(hash)

	if err != nil {
		return fmt.Errorf("invalid CID format %s: %w", hash, err)
	}

	computedCID, err := targetCID.Prefix().Sum(data)

	if err != nil {
		return fmt.Errorf("failed to compute checksum: %w", err)
	}

	if !computedCID.Equals(targetCID) {
		return fmt.Errorf("cryptographic integrity violation: hashes do not match")
	}

	return nil
}
