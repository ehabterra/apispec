# Contributing to APISpec

**APISpec** analyzes your Go code and automatically generates OpenAPI 3.1 specifications (YAML or JSON). It detects routes for popular frameworks (Gin, Echo, Chi, Fiber, net/http), follows call graphs to final handlers, and infers request/response types from real code.

Thank you for your interest in contributing to APISpec! Your contributions, feedback, and help are greatly appreciated.

## Getting Started

### Prerequisites

- Go 1.24+
- Git
- Basic understanding of Go AST and OpenAPI 3.1 specification (helpful but not required)

### Development Setup

1. **Fork and clone the repository**

   ```bash
   git clone https://github.com/your-username/apispec.git
   cd apispec
   ```

2. **Install dependencies**

   ```bash
   make deps
   ```

3. **Build the project**

   ```bash
   make build
   ```

4. **Run tests**

   ```bash
   make test
   ```

## Before Making Changes

Before writing a feature or fixing an issue, please:

1. **Check existing issues** on GitHub to see if your idea or bug is already being discussed
2. **Add comments** to relevant issues if you want to contribute or have questions
3. **Create a new issue** to discuss your proposal if it doesn't exist yet - I'd love to hear your ideas!
4. **Wait for feedback** before starting implementation (if possible) to avoid duplicate work

This helps us work together effectively and ensures contributions align with the project's direction.

## Making Changes

### Workflow

1. **Create a feature branch**
   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/issue-description
   ```

2. **Make your changes**
   - Write clean, well-documented code
   - Follow Go coding standards
   - Add tests when possible

3. **Run tests and linting**
   ```bash
   make test          # Run all tests
   make coverage      # Check test coverage
   make lint          # Run linting checks
   make fmt           # Format code
   ```

4. **Commit your changes**
   ```bash
   git commit -m "Add: brief description of your changes"
   ```

5. **Push and create a Pull Request**
   ```bash
   git push origin feature/your-feature-name
   ```

## Code Standards

- **Write tests when possible**: Tests are helpful, but don't worry if you're not sure how to test something - we can figure it out together
- **Follow Go conventions**: Use `gofmt`, `go vet`, and follow standard Go style
- **Document public APIs**: Add comments for exported functions and types
- **Keep it simple**: Write clear, readable code
- **Don't worry about perfection**: If something isn't quite right, we can iterate on it together. Please add a TODO comment for incomplete parts so we can address them later.

## Testing

- Run tests: `make test`
- Check coverage: `make coverage`
- Run specific tests: `go test ./internal/spec -v`
- Add test cases in `testdata/` for framework-specific features

## Adding Framework Support

To add support for a new web framework:

1. **Update framework detection** in `internal/core/detector.go`
2. **Add default configuration** in `internal/spec/config.go`
3. **Update detection logic** in `cmd/apispec/main.go`
4. **Add test cases** in `testdata/` directory
5. **Update documentation** in `README.md`

If you're unsure about any step, feel free to ask questions or create a draft PR - I'm happy to help!

## Submitting Changes

1. Ensure all tests pass (`make test`)
2. Run linting (`make lint`) - if it fails, don't worry, we can fix it together
3. Update documentation if needed
4. Create a Pull Request with a clear description
5. Reference any related issues

**Note**: PRs don't need to be perfect. If you're stuck or unsure about something, feel free to open a draft PR and ask for help. Collaboration and feedback help us all improve!

## Questions?

If you have questions, need help, or want to discuss something:

- Open an issue on GitHub
- Comment on existing issues
- Don't hesitate to ask - your questions and contributions help make this project better!

## License

By contributing to APISpec, you agree that your contributions will be licensed under the Apache License 2.0, as stated in the [LICENSE](LICENSE) file.

## Code of Conduct

APISpec is committed to fostering a welcoming and inclusive community. Please read and follow our [Code of Conduct](CODE_OF_CONDUCT.md). All contributors are expected to uphold a respectful and collaborative environment.

## A Note from the Maintainer

I'm relatively new to open-source contribution, and I value your help and collaboration. If you notice something that could be improved, have suggestions, or want to help in any way, please reach out. Your contributions, feedback, and expertise are what make this project better for everyone.

Thank you for contributing! 🎉
