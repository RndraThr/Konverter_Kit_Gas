import { render, screen } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import { PlacedMark } from './PlacedMark';

const placement = { x: 0.2, y: 0.1, size: 0.8, rotation: 0 };

test('a mark keeps its size and offset in boxes of different height (BA Perorangan vs other BA)', () => {
  const draw = (boxHeight: number) => {
    const { unmount } = render(<PlacedMark kind="stamp" src="data:," alt="Stempel" placement={placement} boxWidth={400} boxHeight={boxHeight} unit={280} editable={false} selected={false} onSelect={vi.fn()} onChange={vi.fn()} />);
    const style = screen.getByTestId('mark-stamp').style;
    const result = { width: style.width, left: style.left };
    unmount();
    return result;
  };
  const small = draw(155), tall = draw(280);
  expect(small.width).toBe('224px');
  expect(small).toEqual(tall);
});

test('the centre stays inside a short box', () => {
  render(<PlacedMark kind="stamp" src="data:," alt="Stempel" placement={{ ...placement, y: 0.5 }} boxWidth={400} boxHeight={100} unit={280} editable={false} selected={false} onSelect={vi.fn()} onChange={vi.fn()} />);
  expect(screen.getByTestId('mark-stamp').style.top).toBe('calc(50% + 50px)');
});
