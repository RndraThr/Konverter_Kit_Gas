export type Milestone = { stage: 1 | 2 | 3 | 4 | 5; label: string; color: string };

export const MILESTONES: Milestone[] = [
  { stage: 1, label: 'Terjadwal', color: '#c9d1c9' },
  { stage: 2, label: 'Penerima tertaut', color: '#9fcdb0' },
  { stage: 3, label: 'Diserahkan', color: '#5fae7a' },
  { stage: 4, label: 'Bukti lengkap', color: '#2f7d4a' },
  { stage: 5, label: 'BA ditandatangani', color: '#143f24' },
];

// Outline colour for points whose location needs review.
export const REVIEW_COLOR = '#d9711c';
