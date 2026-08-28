// Package access is the authorization checkpoint.
//
// Every read and write of patient data is decided here, against the
// authenticated actor — never against a patient identifier the client supplied,
// which is a claim rather than a permission.
package access
