import { render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'
import { MediaPreviewDialog } from './MediaPreviewDialog'

test('plays video items with native controls instead of the zoomable image', () => {
  render(<MediaPreviewDialog
    items={[{ id: 'media-1', url: '/media/clip.mp4', title: 'clip.mp4', mediaType: 'video' }]}
    index={0}
    onIndexChange={vi.fn()}
  />)

  const video = screen.getByRole('dialog', { name: 'Preview clip.mp4' }).querySelector('video')
  expect(video).toHaveAttribute('src', '/media/clip.mp4')
  expect(video).toHaveAttribute('controls')
  expect(screen.queryByLabelText('Perbesar foto')).not.toBeInTheDocument()
})

test('renders nothing when no item is selected', () => {
  const { container } = render(<MediaPreviewDialog items={[]} index={null} onIndexChange={vi.fn()} />)
  expect(container).toBeEmptyDOMElement()
})
