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

test('forwards input attributes to the input element and layout classes to the wrapper', () => {
  render(<FormField name="username" label="Username" className="sm:col-span-2" maxLength={20} />);

  expect(screen.getByLabelText('Username').parentElement).toHaveClass('sm:col-span-2');
  expect(screen.getByLabelText('Username')).toHaveAttribute('maxLength', '20');
});

test('marks required and optional fields without changing the accessible label', () => {
  render(<><FormField name="phone" label="Nomor HP" requirement="required" /><FormField name="card" label="Nomor kartu" requirement="optional" /></>);

  expect(screen.getByLabelText('Nomor HP')).toHaveAttribute('aria-required', 'true');
  expect(screen.getByText('Wajib')).toBeVisible();
  expect(screen.getByText('Opsional')).toBeVisible();
});
