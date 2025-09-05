#!/bin/bash

# Script to enable test file generation for apispec metadata tests
# This sets the environment variable needed to generate test files

echo "Enabling test file generation for apispec metadata tests..."
echo "This will generate YAML files in internal/spec/tests/ during test runs"
echo ""
echo "Note: These files contain temporary directory paths and should NOT be committed to git"
echo ""

export SWAGEN_WRITE_TEST_FILES=1

echo "Environment variable SWAGEN_WRITE_TEST_FILES=1 has been set"
echo ""
echo "Now you can run tests that will generate files:"
echo "  go test ./internal/metadata -v"
echo ""
echo "To disable test file generation, unset the variable:"
echo "  unset SWAGEN_WRITE_TEST_FILES"
echo ""
echo "Or restart your shell session"
