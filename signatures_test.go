package main

import "testing"

func TestNoneSignatureReturnsInput(t *testing.T) {
	got, err := none("/r100/foo")
	if err != nil {
		t.Fatalf("none returned an error: %v", err)
	}
	if got != "/r100/foo" {
		t.Fatalf("got %q, want input path", got)
	}
}

func TestST3SignatureStripsLegacyPrefix(t *testing.T) {
	input := "/" + "123456789012345678901234" + "r100/foo"

	got, err := ST3sign(input)
	if err != nil {
		t.Fatalf("ST3sign returned an error: %v", err)
	}
	if got != "r100/foo" {
		t.Fatalf("got %q, want stripped path", got)
	}
}

func TestST3SignatureRejectsShortInput(t *testing.T) {
	if _, err := ST3sign("/short"); err == nil {
		t.Fatal("expected short legacy signature to return an error")
	}
}

func TestT3SignatureVerifiesPath(t *testing.T) {
	sig := NewUrlSignature("t3", "secret")
	path := "r100/foo"
	input := "/" + shortHash(path, "secret", 8, 3) + "/" + path

	got, err := sig.Verify(input)
	if err != nil {
		t.Fatalf("T3sign returned an error: %v", err)
	}
	if got != path {
		t.Fatalf("got %q, want %q", got, path)
	}
}

func TestT3SignatureRejectsWrongSignature(t *testing.T) {
	sig := NewUrlSignature("t3", "secret")

	if _, err := sig.Verify("/wrong/r100/foo"); err == nil {
		t.Fatal("expected wrong signature to return an error")
	}
}

func TestT3SignatureRejectsMalformedInput(t *testing.T) {
	sig := NewUrlSignature("t3", "secret")

	if _, err := sig.Verify("missing-slash"); err == nil {
		t.Fatal("expected malformed signature input to return an error")
	}
}

func TestUnknownSignatureMethodCurrentlyDisablesVerification(t *testing.T) {
	sig := NewUrlSignature("unknown", "secret")

	got, err := sig.Verify("/anything")
	if err != nil {
		t.Fatalf("unknown signature verifier returned an error: %v", err)
	}
	if got != "/anything" {
		t.Fatalf("got %q, want input path", got)
	}
}
