package authpassword

import "testing"

func TestHashAndVerify(t *testing.T) {
	hash, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("password was stored in clear text")
	}
	if !IsHash(hash) || !Verify(hash, "correct horse battery staple") {
		t.Fatal("valid password did not verify")
	}
	if Verify(hash, "wrong password") {
		t.Fatal("invalid password verified")
	}
}
