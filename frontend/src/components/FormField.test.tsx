import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import { FormField } from './FormField';

test('associates errors with the input', () => {
  render(<FormField name="email" label="Email" error="Email wajib diisi" />);
  expect(screen.getByLabelText('Email')).toHaveAttribute('aria-invalid', 'true');
  expect(screen.getByLabelText('Email')).toHaveAccessibleDescription('Email wajib diisi');
});

test('associates hints with the input when no error is present', () => {
  render(<FormField name="password" label="Password" hint="Minimal 12 karakter" />);

  expect(screen.getByLabelText('Password')).toHaveAccessibleDescription('Minimal 12 karakter');
});

test('forwards input attributes to the input element', () => {
  render(<FormField name="username" label="Username" className="custom-input" maxLength={20} />);

  expect(screen.getByLabelText('Username')).toHaveClass('custom-input');
  expect(screen.getByLabelText('Username')).toHaveAttribute('maxLength', '20');
});
