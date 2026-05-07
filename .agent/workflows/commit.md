---
description: Automatic commit message generator and fast AI-powered commit for all current changes
---

// turbo-all

This workflow automatically stages all changes, generates a descriptive commit message, and commits them in one go.

### Steps:

1. **Audit Packages**: Run `make audit` to check for package vulnerabilities.
   ```bash
   make audit
   ```
2. **Build Check**: Check if the project is buildable (compilation check only).
   ```bash
   go build ./...
   ```
3. **Stage All Changes**: Automatically stage all modified and new files.
   ```bash
   git add .
   ```
4. **Analyze Changes**: Get the diff of staged changes to understand the context.
   ```bash
   git diff --cached
   ```
5. **Generate & Commit**: Generate a professional message following [Conventional Commits](https://www.conventionalcommits.org/) and execute the commit.
   ```bash
   git commit -m "<ai_generated_message>"
   ```
6. **Push**: Optionally push the changes.
   ```bash
   git push
   ```