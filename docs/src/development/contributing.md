# Contributing

Thank you for your interest in contributing to the Keycloak Operator!

## Code of Conduct

Please be respectful and constructive in all interactions.

## Getting Started

1. Fork the repository
2. Clone your fork
3. Create a feature branch
4. Make your changes
5. Submit a pull request

## Development Workflow

```bash
# Create branch
git checkout -b feature/my-feature

# Make changes
# ...

# Run tests
make test

# Format code
make fmt

# Run linter
make lint

# Commit
git commit -m "Add my feature"

# Push
git push origin feature/my-feature
```

## Pull Request Guidelines

### Before Submitting

- [ ] Code compiles without errors
- [ ] All tests pass
- [ ] Code is formatted (`make fmt`)
- [ ] Linter passes (`make lint`)
- [ ] Documentation updated if needed

### PR Description

Include:
- What the PR does
- Why it's needed
- How to test it

## Code Style

- Follow Go conventions
- Use meaningful variable names
- Add comments for complex logic
- Keep functions focused and small

## Adding a New CRD

1. Define types in `api/v1beta1/`
2. Run `make generate manifests`
3. Implement controller in `internal/controller/`
4. Add Keycloak client methods if needed
5. Write tests
6. Update documentation

## Commit Messages

Use conventional commit format:

```
type(scope): description

feat(realm): add support for realm events
fix(client): handle missing secret gracefully
docs(readme): update installation instructions
```

## Questions?

Open an issue for questions or discussions.
