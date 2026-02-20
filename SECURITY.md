# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| 0.5.x   | :white_check_mark: |
| 0.4.x   | :white_check_mark: |
| < 0.4   | :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability in this project, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

### How to Report

1. **Email:** Send details to [alexei.pape@yandex.ru](mailto:alexei.pape@yandex.ru)
2. **Subject:** `[SECURITY] deckhouse-helm-generator: <brief description>`
3. **Include:**
   - Description of the vulnerability
   - Steps to reproduce
   - Affected versions
   - Potential impact
   - Suggested fix (if any)

### Response Timeline

| Action | Timeline |
|--------|----------|
| Acknowledgment | Within 48 hours |
| Initial assessment | Within 7 days |
| Fix release | Within 30 days (critical: 7 days) |

### What to Expect

- You will receive an acknowledgment within 48 hours
- We will work with you to understand and validate the issue
- A fix will be developed and tested privately
- Credit will be given in the release notes (unless you prefer anonymity)
- A CVE will be requested if applicable

### Scope

The following are in scope:
- Code in this repository
- Dependencies used by this project
- CI/CD pipeline security
- Container images published to GHCR

The following are out of scope:
- Third-party services and infrastructure
- Social engineering attacks
- Denial of service attacks

## Security Practices

This project follows these security practices:

- **Dependency scanning:** Dependabot monitors Go modules and GitHub Actions
- **Secret detection:** Gitleaks runs on every PR
- **Container scanning:** Trivy scans Docker images for vulnerabilities
- **Code review:** All changes require review before merge
- **Branch protection:** Force pushes to `main` are prohibited
