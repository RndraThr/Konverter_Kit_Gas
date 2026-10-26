export type CapturedPhoto = {
  file: File
  /** Real capture time (a backdate only changes the date printed on the watermark). */
  capturedAt: Date
  latitude: number | null
  longitude: number | null
}
