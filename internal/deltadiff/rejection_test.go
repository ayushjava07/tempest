package deltadiff

import (
	"errors"
	"testing"
)

func TestRejectionOfInvalidPatches(t *testing.T) {
	target := map[string]any{
		"name": "Tempest",
		"items": []any{
			"one", "two",
		},
	}

	// 1. Invalid pointer (missing leading slash)
	patchInvalidPtr := Patch{
		{Op: OpReplace, Path: "name", Value: "Invalid"},
	}
	_, err := Apply(target, patchInvalidPtr)
	if !errors.Is(err, ErrInvalidPointer) {
		t.Fatalf("expected ErrInvalidPointer, got %v", err)
	}

	// 2. Non-existent path
	patchMissingPath := Patch{
		{Op: OpReplace, Path: "/non/existent/field", Value: "val"},
	}
	_, err = Apply(target, patchMissingPath)
	if !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("expected ErrPathNotFound, got %v", err)
	}

	// 3. Test condition mismatch
	patchFailedTest := Patch{
		{Op: OpTest, Path: "/name", Value: "WrongName"},
		{Op: OpReplace, Path: "/name", Value: "NewName"},
	}
	_, err = Apply(target, patchFailedTest)
	if !errors.Is(err, ErrTestFailed) {
		t.Fatalf("expected ErrTestFailed, got %v", err)
	}

	// 4. Array remove out of bounds
	patchArrayOOB := Patch{
		{Op: OpRemove, Path: "/items/99"},
	}
	_, err = Apply(target, patchArrayOOB)
	if !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("expected ErrPathNotFound for array index 99, got %v", err)
	}
}

func TestBinaryDecodeRejection(t *testing.T) {
	// Truncated data
	_, err := DecodeBinary([]byte("TPTH"))
	if !errors.Is(err, ErrCorruptBinaryPatch) {
		t.Fatalf("expected ErrCorruptBinaryPatch on truncated data, got %v", err)
	}

	// Bad magic
	badMagic := []byte{'B', 'A', 'A', 'D', 1, 0, 0, 0, 0}
	_, err = DecodeBinary(badMagic)
	if !errors.Is(err, ErrCorruptBinaryPatch) {
		t.Fatalf("expected ErrCorruptBinaryPatch on bad magic, got %v", err)
	}
}
