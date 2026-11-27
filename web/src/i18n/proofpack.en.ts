/** Signed monthly proof pack (TAQ-20, D-074). */
export const proofpackEn = {
  'pp.title': 'Proof pack',
  'pp.q': 'A PDF a client can keep and check: the month in numbers, plus the chain hash and the signature.',
  'pp.who': 'Admin · Supervisor',
  'pp.intro': 'The proof pack lists the month of {site}: attendance, patrols, incidents and warning flags, per day and per guard, with the hash of the record chain and the newest signature. Its last page tells the reader how to check it with garda-verify.',
  'pp.month': 'Month',
  'pp.lang': 'Language',
  'pp.langSite': 'Site language ({lang})',
  'pp.pdf': 'Download proof pack (PDF)',
  'pp.pdfHint': 'Made now from the current records.',
  'pp.kept': 'Download the kept copy',
  'pp.keptHint': 'The copy the system made on the first days of the month.',
  'pp.bundle': 'Download export bundle (ZIP)',
  'pp.bundleHint': 'The file the reader checks with garda-verify. It contains the records of the month, so share it only with people who may see them.',
  'pp.unsigned': 'If the PDF says UNSIGNED TAIL, no signature made after the month ended covers the last records yet. Make it again after the next signature; it is never shown as fully verified.',
  'pp.openMonth': 'This month is not over yet, so the pack cannot be signed after its end.',
} as const
