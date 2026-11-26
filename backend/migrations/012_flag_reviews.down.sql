-- Reverts 012. Review marks are lost; the flags themselves stay on the records.
DROP TABLE IF EXISTS flag_reviews;
DROP FUNCTION IF EXISTS flag_reviews_immutable();
