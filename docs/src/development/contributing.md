# Contributing

Thank you for your interest in contributing to the Keycloak Operator!

## Code of Conduct

Please be respectful and constructive in all interactions.

## How to Contribute

### Reporting Issues

- Search existing issues first
- Provide clear reproduction steps
- Include relevant logs and configuration

### Submitting Changes

1. Fork the repository
2. Create a feature branch:
   ```bash
   git checkout -b feature/my-feature
   ```

3. Make your changes following the code style

4. Add tests for new functionality

5. Run checks:
   ```bash
   make fmt
   make vet
   make lint
   make test
   ```

6. Commit with a clear message:
   ```bash
   git commit -m "feat: add support for X"
   ```

7. Push and create a Pull Request

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation only
- `refactor:` Code change without feature/fix
- `test:` Adding tests
- `chore:` Maintenance

### Code Style

- Follow standard Go conventions
- Use `gofmt` and `golangci-lint`
- Add comments for exported types/functions
- Keep functions focused and small

### Testing Requirements

- Unit tests for new logic
- E2E tests for new CRD features and Keycloak interactions

## Development Setup

See [Local Setup](./local-setup.md) for environment setup.

## Pull Request Process

1. Ensure all tests pass
2. Update documentation if needed
3. Request review from maintainers
4. Address feedback
5. Squash commits if requested

## Getting Help

- Open an issue for questions
- Check existing documentation
- Review similar PRs for patterns

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
