package indexer

import (
	"testing"
)

const sampleTS = `
import { Injectable } from '@angular/core'
import fs from 'fs'

export function greet(name: string): string {
  return "hello " + name
}

const add = (a: number, b: number): number => a + b

const multiply = async (x: number) => {
  return x * 2
}

export class UserService {
  private users: string[] = []

  constructor(private db: Database) {}

  async getUser(id: string): Promise<User> {
    return this.db.find(id)
  }
}

export interface Repository<T> {
  find(id: string): Promise<T>
  save(item: T): Promise<void>
}

export type UserId = string

export enum Status {
  Active = 'active',
  Inactive = 'inactive',
}
`

const sampleJS = `
import path from 'path'

function hello(name) {
  return 'hi ' + name
}

const double = (x) => x * 2

const triple = async (x) => {
  return x * 3
}

class Animal {
  constructor(name) {
    this.name = name
  }
}
`

func TestParseJSFileTypeScript(t *testing.T) {
	decls, info, err := ParseJSFile("services/user.ts", sampleTS)
	if err != nil {
		t.Fatalf("ParseJSFile error: %v", err)
	}

	if info.Package != "user" {
		t.Errorf("package = %q, want %q", info.Package, "user")
	}
	if info.IsTest {
		t.Error("IsTest should be false")
	}

	if len(info.Imports) == 0 {
		t.Error("expected at least one import")
	}
	wantImport := "@angular/core"
	found := false
	for _, imp := range info.Imports {
		if imp == wantImport {
			found = true
		}
	}
	if !found {
		t.Errorf("expected import %q in %v", wantImport, info.Imports)
	}

	byName := make(map[string]ParsedDecl, len(decls))
	for _, d := range decls {
		byName[d.Name] = d
	}

	tests := []struct {
		name string
		kind DeclKind
	}{
		{"greet", DeclFunc},
		{"add", DeclFunc},
		{"multiply", DeclFunc},
		{"UserService", DeclClass},
		{"Repository", DeclInterface},
		{"UserId", DeclType},
		{"Status", DeclEnum},
	}
	for _, tc := range tests {
		d, ok := byName[tc.name]
		if !ok {
			t.Errorf("missing declaration %q", tc.name)
			continue
		}
		if d.Kind != tc.kind {
			t.Errorf("%q: kind = %v, want %v", tc.name, d.Kind, tc.kind)
		}
		if d.StartLine <= 0 {
			t.Errorf("%q: StartLine = %d, want > 0", tc.name, d.StartLine)
		}
		if d.EndLine < d.StartLine {
			t.Errorf("%q: EndLine %d < StartLine %d", tc.name, d.EndLine, d.StartLine)
		}
	}
}

func TestParseJSFileJavaScript(t *testing.T) {
	decls, info, err := ParseJSFile("utils/helpers.js", sampleJS)
	if err != nil {
		t.Fatalf("ParseJSFile error: %v", err)
	}

	if info.Package != "helpers" {
		t.Errorf("package = %q, want %q", info.Package, "helpers")
	}

	byName := make(map[string]ParsedDecl, len(decls))
	for _, d := range decls {
		byName[d.Name] = d
	}

	tests := []struct {
		name string
		kind DeclKind
	}{
		{"hello", DeclFunc},
		{"double", DeclFunc},
		{"triple", DeclFunc},
		{"Animal", DeclClass},
	}
	for _, tc := range tests {
		d, ok := byName[tc.name]
		if !ok {
			t.Errorf("missing declaration %q", tc.name)
			continue
		}
		if d.Kind != tc.kind {
			t.Errorf("%q: kind = %v, want %v", tc.name, d.Kind, tc.kind)
		}
	}
}

func TestParseJSFileTestDetection(t *testing.T) {
	_, info, _ := ParseJSFile("services/user.test.ts", "")
	if !info.IsTest {
		t.Error("expected IsTest = true for *.test.ts")
	}

	_, info2, _ := ParseJSFile("services/user.spec.js", "")
	if !info2.IsTest {
		t.Error("expected IsTest = true for *.spec.js")
	}
}

func TestIsJSLike(t *testing.T) {
	for _, path := range []string{"a.js", "a.jsx", "a.ts", "a.tsx", "a.mjs", "a.cjs"} {
		if !IsJSLike(path) {
			t.Errorf("IsJSLike(%q) = false, want true", path)
		}
	}
	for _, path := range []string{"a.go", "a.py", "a.rs", "a.json"} {
		if IsJSLike(path) {
			t.Errorf("IsJSLike(%q) = true, want false", path)
		}
	}
}

func TestParseJSFileTSX(t *testing.T) {
	src := `
import React from 'react'

export function Button({ onClick }: ButtonProps) {
  return <button onClick={onClick}>Click</button>
}

export const Card: React.FC<CardProps> = ({ title }) => {
  return <div>{title}</div>
}
`
	decls, info, err := ParseJSFile("components/Button.tsx", src)
	if err != nil {
		t.Fatalf("ParseJSFile TSX error: %v", err)
	}
	if info.Package != "Button" {
		t.Errorf("package = %q, want %q", info.Package, "Button")
	}

	byName := make(map[string]ParsedDecl)
	for _, d := range decls {
		byName[d.Name] = d
	}

	if _, ok := byName["Button"]; !ok {
		t.Error("missing Button function declaration")
	}
	if _, ok := byName["Card"]; !ok {
		t.Error("missing Card arrow function declaration")
	}
}

func TestParseJSFileNamespace(t *testing.T) {
	src := `
namespace Http {
  export function get(url: string) {}
  export class Client {
    post(url: string) {}
  }
}

module Auth {
  export function login() {}
}
`
	decls, _, err := ParseJSFile("lib/http.ts", src)
	if err != nil {
		t.Fatalf("ParseJSFile namespace error: %v", err)
	}

	byName := make(map[string]ParsedDecl)
	for _, d := range decls {
		byName[d.Name] = d
	}

	for _, want := range []string{"get", "Client", "login"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("missing declaration %q from namespace/module; got: %v", want, decls)
		}
	}
}
