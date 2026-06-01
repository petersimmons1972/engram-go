-- 024_canonicalize_embedder_name_bge_m3.sql
--
-- Canonicalize historical llama.cpp GGUF model IDs in project_meta so
-- embedder_name metadata no longer depends on runtime alias maps.
--
-- Safe to run multiple times: UPDATE targets only the legacy value.
UPDATE project_meta
SET value = 'BAAI/bge-m3'
WHERE key = 'embedder_name'
  AND value = 'bge-m3-Q8_0.gguf';
