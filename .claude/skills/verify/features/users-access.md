# Users & access

An administrator decides who can sign in to the dashboard, with which role and for which sites. A new user gets a temporary password that is shown once.

## Sub-features

- `users-create` adds a user with roles, a site limit and a temporary password.
- `users-field-error` shows an API error next to the field, for example a taken username.
- `users-self-protect` locks the signed-in admin's own admin role, status and password reset.
- `users-reset-enable` re-enables a disabled user and resets a password.
- `users-tablet-account` creates a tablet account and shows its sign-in details once.

## How to get to it (user POV)

- Choose `Pengguna & akses` in the main navigation (`/users`), then the `Tambah pengguna` button.
- Open `/users?new=1` to land directly in the `Tambah pengguna` dialog.
- Choose `Kelola — <name>` on a row to open that user's drawer.

## Driving it with verify.mjs

Preconditions:

- `DOCTOR OK` on the instance.
- The role is `admin`. No user is named `spv.sari`.

Run `node .claude/skills/verify/verify.mjs drive .claude/skills/verify/journeys/users-create.mjs`.

- **Open the list.** `openAs('admin', '/users')`. The heading `Pengguna & akses` shows and `world()` does not contain `spv.sari`.
- **Open the dialog.** Click the button `Tambah pengguna`. The dialog `Tambah pengguna` appears.
- **Fill the form.** Fill `Nama pengguna` with `Spv.Sari` and `Nama depan` with `Sari`, check the checkbox `Supervisor`, the radio `Hanya lokasi berikut` and the radio `Atur kata sandi sementara sekarang`.
- **Create.** Click `Buat pengguna`. The dialog `Sari sudah dibuat` shows `Hanya ditampilkan sekali` and a 16-character value in the textbox `Kata sandi sementara`.
- **Confirm in the list.** Click `Selesai, sudah saya salin`, fill `Cari pengguna` with `spv.sari`. One row remains, with `Gudang Cikarang` and `Wajib ganti kata sandi`.
- **Proof.** `world()` contains `spv.sari`; screenshots `01` to `04` show the list before, the form, the password dialog and the filtered list.

## Gotchas

- The username is lower-cased on save: type `Spv.Sari`, assert `spv.sari`.
- The search filters after a short delay. Wait for another row (`admin.dimas`) to disappear before the screenshot.
- The temporary password is random. Assert its shape, never its value, and do not copy it into notes.
- Not driven: `users-field-error`, `users-self-protect`, `users-reset-enable`, `users-tablet-account`, and the `/users?new=1` entry point. `web/e2e/people.spec.ts` has working selectors for all of them.
- In the real stack these actions create Keycloak users. The demo proves none of that.
