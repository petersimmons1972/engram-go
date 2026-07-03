BEGIN;

ALTER TABLE atoms
    ADD COLUMN IF NOT EXISTS observed_at TIMESTAMPTZ;

UPDATE atoms
SET observed_at = created_at
WHERE observed_at IS NULL;

ALTER TABLE atoms
    DROP CONSTRAINT IF EXISTS atoms_atom_type_check;

ALTER TABLE atoms
    ADD CONSTRAINT atoms_atom_type_check CHECK (atom_type IN (
        'preference', 'profile', 'status_change', 'fact', 'event', 'attribute', 'relationship'
    ));

CREATE INDEX IF NOT EXISTS idx_atoms_project_type_observed_at
    ON atoms (project, atom_type, observed_at DESC)
    WHERE valid_to IS NULL;

COMMIT;
