# Contributing to GPUHealth

First and foremost, thank you for considering contributing to GPUHealth! We appreciate the time and effort you're putting into helping improve our project. This guide outlines the process and standards we expect from contributors.

## Issue Tracking

All enhancement, bugfix, or change requests must begin with the creation of a GPUHealth Issue Request. The issue request must be reviewed by GPUHealth engineers and approved prior to code review.

When creating an issue:
- Use clear and descriptive titles
- Provide detailed descriptions of the problem or enhancement request
- Include steps to reproduce for bug reports
- Attach relevant logs, error messages, or screenshots when applicable
- Label the issue appropriately (bug, enhancement, documentation, etc.)

## Development

First clone the source code from Github

```bash
git clone https://github.com/leptonai/gpuhealth.git
```

Use `go` to build `gpuhealth` from source

```bash
cd gpuhealth
make all

./bin/gpuhealth -h
```

## Testing

We highly recommend writing tests for new features or bug fixes and ensure all tests passing before submitting a PR.

To run all existing tests locally, simply run

```bash
./scripts/tests-unit.sh
./scripts/tests-e2e.sh
```

## Developer Certificate of Origin (DCO)

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.
```

```
Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

### How to Sign Your Work

To sign your work and agree to the DCO, you must add a sign-off to every git commit. This is done by using the `-s` flag when committing:

```bash
git commit -s -m "Your commit message"
```

This will append a line that looks like:

```
Signed-off-by: Your Name <your.email@example.com>
```

You must use your real name and a valid email address. Anonymous contributions or contributions under pseudonyms are not accepted.

If you forget to add the sign-off to a commit, you can amend it:

```bash
git commit --amend --signoff
```

For more information about the DCO, see: https://developercertificate.org/

## Pull Request Process

1. **Fork the Repository**: Create a personal fork of the GPUHealth repository on GitHub.

2. **Create a Feature Branch**: Create a new branch for your changes from the main branch:
   ```bash
   git checkout -b feature/your-feature-name
   ```

3. **Make Your Changes**: Implement your changes following the coding standards outlined below.

4. **Test Your Changes**: Ensure all tests pass and add new tests for your changes if applicable.

5. **Commit with Sign-off**: Make sure all commits are signed off according to the DCO requirements.

6. **Submit Pull Request**: Create a pull request against the main branch with:
   - Clear title and description
   - Reference to the related issue
   - Summary of changes made
   - Any breaking changes highlighted

7. **Code Review**: Address any feedback from maintainers during the review process.

## Coding Standards

Ensure your code is clean, readable, and well-commented. We use the following tools and guidelines:

### Go Code Standards
- Follow standard Go conventions and idioms
- Use `gofmt` for code formatting
- Use `golangci-lint` for linting

To run linting locally:

```bash
golangci-lint run
```

### General Guidelines
- Write clear, descriptive commit messages
- Keep commits focused and atomic
- Add comments for complex logic
- Update documentation when adding new features
- Ensure backward compatibility when possible
