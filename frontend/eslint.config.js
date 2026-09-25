// eslint.config.js
import js from '@eslint/js';
import globals from 'globals';
import reactHooks from 'eslint-plugin-react-hooks';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  // Ignore build output and test artefacts.
  { ignores: ['../web/static/app/**', 'node_modules/**', 'test-results/**'] },

  // Base JS recommended rules for all files.
  js.configs.recommended,

  // Application TypeScript files — recommended rules with project-aware parsing.
  {
    files: ['src/**/*.{ts,tsx}'],
    extends: [
      ...tseslint.configs.recommended,
    ],
    languageOptions: {
      globals: { ...globals.browser },
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    plugins: {
      'react-hooks': reactHooks,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      // Ternary expressions used purely for side effects are a deliberate pattern
      // throughout the codebase (e.g. `value ? params.set(k,v) : params.delete(k)`).
      '@typescript-eslint/no-unused-expressions': ['error', { allowTernary: true, allowShortCircuit: true }],
      // Downgrade to warn for now to allow gradual adoption without blocking CI.
      // Promote to error once the codebase has zero warnings.
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      '@typescript-eslint/no-explicit-any': 'warn',
    },
  },

  // E2E test files — Node globals, relax type checking.
  {
    files: ['e2e/**/*.{ts,tsx}'],
    extends: [...tseslint.configs.recommended],
    languageOptions: {
      globals: { ...globals.node },
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
  },
);
