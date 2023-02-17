ALTER TABLE options ADD pin INT(4) NOT NULL DEFAULT -1;
ALTER TABLE options ALTER operations SET DEFAULT '+';
ALTER TABLE options ALTER fractions SET DEFAULT 0;
ALTER TABLE options ALTER negatives SET DEFAULT 0;
ALTER TABLE options ALTER target_difficulty SET DEFAULT 3;
