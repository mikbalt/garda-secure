/** Export formats of the API (D-045): every report CSV + XLSX, the incident log also PDF. */
export type ExportFormat = 'csv' | 'xlsx' | 'pdf'

/** "report-2026-10.csv" → "report-2026-10.xlsx". */
export function exportFilename(base: string, format: ExportFormat): string {
  return `${base.replace(/\.(csv|xlsx|pdf)$/i, '')}.${format}`
}
