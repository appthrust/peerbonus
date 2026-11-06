ALTER TABLE members ADD COLUMN inbox_seen_at timestamptz;

UPDATE program SET values = '[{"key":"thanks","label":"感謝","description":"助けてもらった、支えてもらった"},{"key":"challenge","label":"挑戦","description":"新しいことに踏み出した"},{"key":"teamwork","label":"協力","description":"チームの壁を越えて動いた"},{"key":"learning","label":"学び","description":"知識や経験を分けてくれた"},{"key":"customer","label":"顧客志向","description":"お客様の価値を最優先した"}]'::jsonb WHERE id = 1 AND values = '[]'::jsonb;
