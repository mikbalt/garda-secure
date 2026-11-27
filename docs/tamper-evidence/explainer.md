# Garda records: what is tamper-evident, and what is not

*Bahasa Indonesia: [explainer.id.md](explainer.id.md)* · For a client or sales call. Each tag like **[G1]** points to its evidence in [claims.md](claims.md).

Garda's records are **tamper-evident, anchored outside the app**. That means: if a record is changed or deleted afterwards,
a check run with the client's own fingerprint shows it, for records before the last signature, and the check does not depend on our word. It does **not** mean a record cannot be changed.

## What Garda records

Every clock-in and clock-out, every checkpoint scan on a patrol, every incident and every later correction to an incident **[G1]**.
Checkpoint QR codes carry a time window. A scan with an old or wrong code, a late sync or a tablet clock that looks off is **kept and
flagged** for a supervisor to review; it is not thrown away. By design, marking a flag as reviewed is stored separately and does not write to the record (decision D-070; no test reads the chain after a review) **[F1]**.

## How a record is chained and signed

1. Each time one of these records is saved, Garda adds a line to the site's **chain** in the same step. The line holds a fingerprint of the
   record and of the line before it **[G1]**.
2. If a record or a line is later edited or deleted, even directly in the database, the chain stops matching from that point, and the check
   names the first record that changed **[G2]**.
3. A correction made through the app (a supervisor's fix, a clock-out) is not tampering. It adds a new line; the old lines stay, so you can see that a change was made and when. The earlier values are not kept **[G3]**.
4. About every hour, and at the end of each site day, a **separate program, the signer**, signs the newest line with a key the app cannot
   use. It runs as its own process with its own key file and its own database account **[G4]**.
5. The signer refuses to sign a shorter or different history. It also keeps its own list of what it signed, on its own disk, so wiping
   the database copy does not make it forget **[G6]**.
6. A history that was already signed and then rewritten and re-linked so that it looks tidy no longer matches the signatures, and the check fails **[G5]**. Records after the last signature can still be rewritten until the next signing (see the box).

## What a client can check on their own

With the small program `garda-verify`, an export file (a `.zip`) and the **signer key fingerprint they were given at installation**, a
client can run the check on their own computer: no account, no internet, no Garda system **[G7]**. The answer is `VALID`, or the first
record that no longer matches. A wrong fingerprint fails **[G7]**. The tool does not say plain `VALID` for an empty file, for records
after the last signature, for records whose later change is outside the file, or for a date range it was not told to hold the file to **[G8]**.
The monthly proof-pack PDF repeats this check and prints the steps, but it is the server checking itself. The client's own check is
the one that counts **[P1]**.

## What this does NOT guarantee

| | |
|---|---|
| **Administrator (root) access to the server.** | Anyone with root on the server that runs Garda can also read the signer key file and sign a rewritten history. The client's own fingerprint, earlier exports kept by the client and key changes made in the client's presence limit this **[L1]**. |
| **Records after the last signature.** | An export asks the signer for a fresh signature when it is made, so the newest records are covered. If the signer does not answer or refuses, the file still comes, the tool reports `UNSIGNED TAIL` with the reason and does not call it valid; a file cut short looks the same **[L2]**. |
| **An old export shown as current.** | A genuine old file stays valid forever for what it holds. Look at its export date and signature date **[L3]**. |
| **Figures in the proof-pack PDF.** | Attendance, patrol, incident and flag totals are read from the database when the PDF is made. They are not signed, and the PDF says so **[L4]**. |
| **Photo of a printed QR code.** | A sticker cannot change. A copy can be replayed until the code's window ends (30 days by default) plus about 24 hours **[L5]**. |
| **A guard who is offline and lies about the time.** | A tablet that is offline, with its clock set back, can replay that photo until the same limit. Flags and the GPS and photo checks raise a signal; the scan is not blocked (decision D-072) **[L6]**. |
| **Whether what was entered is true.** | The chain shows a record was not changed after it was saved, not that it was true when entered. Someone using the app itself can still add a wrong new line, and it shows as a new line with a time **[L7]**. |
| **The fingerprint you hold.** | Only a fingerprint the client got out of band counts. The one printed in the PDF is the server's own claim **[L8]**. |

Also: records that already existed when the chain was switched on are vouched for only as they were that day **[L9]**, and every export is logged (who, which site and range, when, how big), but what is done with the file afterwards is not **[L10]**.

## Words to use and to avoid

Use: "tamper-evident", "anchored outside the app", "you can check it yourself". Avoid: "tamper-proof", "cannot be changed",
"no need to trust us". See [claims.md](claims.md).
