export type PageItem = number | 'ellipsis-start' | 'ellipsis-end';

export function buildPageItems(currentPage: number, totalPages: number): PageItem[] {
  if (totalPages < 1) return [];
  if (totalPages <= 7) return Array.from({ length: totalPages }, (_, index) => index + 1);

  const current = Math.min(Math.max(currentPage, 1), totalPages);
  if (current <= 4) return [1, 2, 3, 4, 5, 'ellipsis-end', totalPages];
  if (current >= totalPages - 3) {
    return [1, 'ellipsis-start', totalPages - 4, totalPages - 3, totalPages - 2, totalPages - 1, totalPages];
  }
  return [1, 'ellipsis-start', current - 1, current, current + 1, 'ellipsis-end', totalPages];
}
