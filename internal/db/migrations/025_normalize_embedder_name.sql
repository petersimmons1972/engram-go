-- Normalize stale embedder_name values to canonical form.
-- Covers aliases such as "BAAI/bge-m3:latest" that were written before
-- canonicalEmbedderName() was applied at registration time (#911).
UPDATE project_meta
SET value = 'BAAI/bge-m3'
WHERE key = 'embedder_name'
  AND value != 'BAAI/bge-m3'
  AND value ILIKE '%bge-m3%';
