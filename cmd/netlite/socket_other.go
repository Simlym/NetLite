//go:build !linux

package main

func isStaleSocket(error) bool { return false }
