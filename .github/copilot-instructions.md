# GitHub Copilot Instructions

## Environment development and deployment
   - Use Go 1.24 or later.
   - Use Go Modules for dependency management.
   - Follow semantic versioning for releases.
   - IDE: Use VSCode with Go extension.
   - OS: Development on Windows, deployment on Linux.
   - CI/CD: Use GitHub Actions for continuous integration and deployment.
   - Containerization: Use Docker for containerizing the application.
   - Database: Use BadgerDB for local storage.
   - Testing: Use Go's built-in testing framework and Testify for assertions.

## Project Coding and Communication Conventions
1. **Language in Code**
   - All code, including comments, documentation (GoDoc), and identifiers (variables, functions, etc.), must be written in English.
   - GoDoc and comments must be in English, following these rules:
        ### Rules for GoDoc (package, func, type comments)
            - **First Sentence Summary**: The first sentence must be a complete summary that starts with the name of the documented element (e.g., `// MyFunction does...`). This sentence must end with a period.
            - **Detailed Description**: Any detailed description paragraphs, if present, must be separated from the summary sentence by a blank line.
            - **Active Voice**: Use the active voice to explain the role, responsibility, and design rationale of the element.

        ### Rules for Inline Comments (inside functions)
            - **Placement**: Place the comment on the line **immediately above** the code block it explains.
            - **Complete Sentences**: Write comments as complete sentences, starting with a capital letter and ending with a period.
            - **Explain the "Why"**: Focus on explaining the **"why" (intent)** of the code, not the "what" (which is already clear from the code itself).
                * **Bad**: `// Loop through items`
                * **Good**: `// Process each item to calculate the total score.`

        ### Line Wrapping
            - Wrap all comments and GoDoc lines at **120 characters** to ensure readability across different screen sizes and editors.

        ### Package level
            - Write GoDoc for every package in a `doc.go` file within that package's directory.

2. **Chat Window Language Requirement**
    - All communication, responses, and discussions in this chat window must be conducted entirely in Vietnamese, regardless of context or topic.

3. **Documentation and Style Consistency**
    - Code must be accompanied by detailed comments and comprehensive GoDoc. A consistent coding style must be maintained across all packages throughout the entire repository.

4. **Change Implementation Process**
    - Before implementing any changes, you must first thoroughly understand the existing codebase. This is to ensure you leverage current functionalities and that new code aligns with the established architecture and workflow.
    - Must run all tooling checks (e.g., linters, formatters, tests, build) before submitting any code changes.

5. **Production-Grade Standards**
    - This project is being developed for a large-scale production environment. Therefore, it demands the highest standards of correctness, reliability, security, and performance.

6. Use "Copyright 2025 Nguyen Thanh Phuong. All rights reserved." as the copyright notice in all new code files.