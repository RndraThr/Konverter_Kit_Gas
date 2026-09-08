import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, test } from 'vitest';
import { LoginPage } from './LoginPage';

afterEach(() => {
  history.replaceState({}, '', '/');
});

test('keeps field imagery, brands, and submission feedback accessible', async () => {
  history.replaceState({}, '', '/login?error=invalid');
  const user = userEvent.setup();
  render(<LoginPage />);

  expect(screen.getByRole('main')).toBeInTheDocument();
  expect(screen.getByRole('form', { name: 'Masuk ke dashboard' })).toBeInTheDocument();
  expect(screen.getByAltText('Ergas')).toBeInTheDocument();
  expect(screen.getByAltText('PT Kian Santang Mulitama Tbk')).toBeInTheDocument();
  expect(screen.getByRole('alert')).toHaveTextContent('Email/username atau password tidak sesuai.');

  await user.type(screen.getByLabelText('Email atau username'), 'admin');
  await user.type(screen.getByLabelText('Password'), 'password-rahasia');

  expect(screen.getByRole('button', { name: 'Masuk' })).toBeEnabled();
});
