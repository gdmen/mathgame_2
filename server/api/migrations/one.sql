ALTER TABLE options ADD pin VARCHAR(4) NOT NULL DEFAULT '';
ALTER TABLE options ALTER operations SET DEFAULT '+';
ALTER TABLE options ALTER fractions SET DEFAULT 0;
ALTER TABLE options ALTER negatives SET DEFAULT 0;
ALTER TABLE options ALTER target_difficulty SET DEFAULT 3;