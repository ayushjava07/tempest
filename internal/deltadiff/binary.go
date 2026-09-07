package deltadiff

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	BinaryMagicHeader = [4]byte{'T', 'P', 'T', 'H'}
	BinaryVersion     = byte(1)

	ErrCorruptBinaryPatch = errors.New("deltadiff: corrupt or truncated binary patch data")
	ErrUnsupportedVersion = errors.New("deltadiff: unsupported binary patch version")
)

const (
	binOpAdd byte = iota
	binOpRemove
	binOpReplace
	binOpMove
	binOpCopy
	binOpTest
)

// EncodeBinary serializes a Patch into a compact binary representation.
func EncodeBinary(patch Patch) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Header: [4]byte magic + [1]byte version
	buf.Write(BinaryMagicHeader[:])
	buf.WriteByte(BinaryVersion)

	// Op count: uint32
	if err := binary.Write(buf, binary.BigEndian, uint32(len(patch))); err != nil {
		return nil, err
	}

	for _, op := range patch {
		opCode := opTypeToByte(op.Op)
		buf.WriteByte(opCode)

		// Path: uint16 len + bytes
		pathBytes := []byte(op.Path)
		if err := binary.Write(buf, binary.BigEndian, uint16(len(pathBytes))); err != nil {
			return nil, err
		}
		buf.Write(pathBytes)

		// From (if move or copy): uint16 len + bytes
		fromBytes := []byte(op.From)
		if err := binary.Write(buf, binary.BigEndian, uint16(len(fromBytes))); err != nil {
			return nil, err
		}
		if len(fromBytes) > 0 {
			buf.Write(fromBytes)
		}

		// Value: uint32 len + JSON bytes
		var valBytes []byte
		if op.Value != nil {
			var err error
			valBytes, err = json.Marshal(op.Value)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal op value: %w", err)
			}
		}

		if err := binary.Write(buf, binary.BigEndian, uint32(len(valBytes))); err != nil {
			return nil, err
		}
		if len(valBytes) > 0 {
			buf.Write(valBytes)
		}
	}

	return buf.Bytes(), nil
}

// DecodeBinary parses a Patch from binary representation.
func DecodeBinary(data []byte) (Patch, error) {
	if len(data) < 9 { // 4 magic + 1 ver + 4 count
		return nil, ErrCorruptBinaryPatch
	}

	if !bytes.Equal(data[:4], BinaryMagicHeader[:]) {
		return nil, fmt.Errorf("%w: invalid magic header", ErrCorruptBinaryPatch)
	}

	if data[4] != BinaryVersion {
		return nil, fmt.Errorf("%w: got %d want %d", ErrUnsupportedVersion, data[4], BinaryVersion)
	}

	reader := bytes.NewReader(data[5:])
	var count uint32
	if err := binary.Read(reader, binary.BigEndian, &count); err != nil {
		return nil, ErrCorruptBinaryPatch
	}

	ops := make(Patch, 0, count)
	for i := uint32(0); i < count; i++ {
		opCode, err := reader.ReadByte()
		if err != nil {
			return nil, ErrCorruptBinaryPatch
		}

		var pathLen uint16
		if err := binary.Read(reader, binary.BigEndian, &pathLen); err != nil {
			return nil, ErrCorruptBinaryPatch
		}
		pathBytes := make([]byte, pathLen)
		if _, err := reader.Read(pathBytes); err != nil {
			return nil, ErrCorruptBinaryPatch
		}

		var fromLen uint16
		if err := binary.Read(reader, binary.BigEndian, &fromLen); err != nil {
			return nil, ErrCorruptBinaryPatch
		}
		var fromStr string
		if fromLen > 0 {
			fromBytes := make([]byte, fromLen)
			if _, err := reader.Read(fromBytes); err != nil {
				return nil, ErrCorruptBinaryPatch
			}
			fromStr = string(fromBytes)
		}

		var valLen uint32
		if err := binary.Read(reader, binary.BigEndian, &valLen); err != nil {
			return nil, ErrCorruptBinaryPatch
		}
		var val any
		if valLen > 0 {
			valBytes := make([]byte, valLen)
			if _, err := reader.Read(valBytes); err != nil {
				return nil, ErrCorruptBinaryPatch
			}
			if err := json.Unmarshal(valBytes, &val); err != nil {
				return nil, fmt.Errorf("%w: malformed value json: %v", ErrCorruptBinaryPatch, err)
			}
		}

		ops = append(ops, PatchOperation{
			Op:    byteToOpType(opCode),
			Path:  string(pathBytes),
			From:  fromStr,
			Value: val,
		})
	}

	return ops, nil
}

func opTypeToByte(op OpType) byte {
	switch op {
	case OpAdd:
		return binOpAdd
	case OpRemove:
		return binOpRemove
	case OpReplace:
		return binOpReplace
	case OpMove:
		return binOpMove
	case OpCopy:
		return binOpCopy
	case OpTest:
		return binOpTest
	default:
		return 255
	}
}

func byteToOpType(b byte) OpType {
	switch b {
	case binOpAdd:
		return OpAdd
	case binOpRemove:
		return OpRemove
	case binOpReplace:
		return OpReplace
	case binOpMove:
		return OpMove
	case binOpCopy:
		return OpCopy
	case binOpTest:
		return OpTest
	default:
		return OpType("unknown")
	}
}
