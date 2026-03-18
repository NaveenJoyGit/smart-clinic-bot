-- 001_sample_clinic.sql
-- Sample data for "Bright Smile Dental Clinic", New Delhi.
-- Run AFTER all migrations.  Idempotent via ON CONFLICT DO NOTHING.

BEGIN;

-- ── Clinic ──────────────────────────────────────────────────────────────────
INSERT INTO clinics (
    id, name, slug, address, city, state, country,
    phone, email, website,
    timings,
    receptionist_name, receptionist_phone, receptionist_email
) VALUES (
    'a1b2c3d4-0000-0000-0000-000000000001',
    'Bright Smile Dental Clinic',
    'bright-smile-delhi',
    '14, Connaught Place, Block E',
    'New Delhi', 'Delhi', 'India',
    '+91-11-4567-8900',
    'hello@brightsmile.in',
    'https://brightsmile.in',
    '{
        "monday":    {"open": "09:00", "close": "20:00"},
        "tuesday":   {"open": "09:00", "close": "20:00"},
        "wednesday": {"open": "09:00", "close": "20:00"},
        "thursday":  {"open": "09:00", "close": "20:00"},
        "friday":    {"open": "09:00", "close": "20:00"},
        "saturday":  {"open": "10:00", "close": "17:00"},
        "sunday":    null
    }',
    'Priya Sharma', '+91-98765-43210', 'priya@brightsmile.in'
)
ON CONFLICT (slug) DO NOTHING;


-- ── Services ────────────────────────────────────────────────────────────────
INSERT INTO clinic_services
    (id, clinic_id, name, category, description, price_min, price_max, price_note, duration_minutes)
VALUES
    (
        'b1000001-0000-0000-0000-000000000001',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'Dental Check-up & Cleaning',
        'preventive',
        'Full oral examination with digital X-rays and professional cleaning (scaling & polishing).',
        500, 1500,
        'Price varies based on tartar build-up level.',
        45
    ),
    (
        'b1000001-0000-0000-0000-000000000002',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'Teeth Whitening',
        'cosmetic',
        'In-chair laser whitening treatment for up to 8 shades brighter smile in one session.',
        6000, 12000,
        'Single session; touch-up kit included.',
        90
    ),
    (
        'b1000001-0000-0000-0000-000000000003',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'Dental Implants',
        'restorative',
        'Titanium implant with ceramic crown; replaces missing teeth permanently.',
        25000, 60000,
        'Per implant; bone grafting charged separately if required.',
        NULL
    ),
    (
        'b1000001-0000-0000-0000-000000000004',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'Braces (Metal)',
        'orthodontics',
        'Traditional stainless steel braces for teeth alignment; includes all adjustment visits.',
        18000, 30000,
        'Total cost for full treatment (12-24 months).',
        NULL
    ),
    (
        'b1000001-0000-0000-0000-000000000005',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'Invisible Aligners',
        'orthodontics',
        'Clear removable aligners (Invisalign-compatible) for discreet teeth straightening.',
        55000, 120000,
        'Full treatment cost; includes refinements.',
        NULL
    ),
    (
        'b1000001-0000-0000-0000-000000000006',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'Root Canal Treatment',
        'restorative',
        'Single sitting RCT with bio-ceramic sealer and crown restoration.',
        3500, 9000,
        'Per tooth; crown cost additional.',
        60
    ),
    (
        'b1000001-0000-0000-0000-000000000007',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'Tooth Extraction',
        'surgical',
        'Simple or surgical extraction under local anaesthesia.',
        500, 3000,
        'Surgical extraction (impacted wisdom tooth) at higher end.',
        30
    ),
    (
        'b1000001-0000-0000-0000-000000000008',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'Porcelain Veneers',
        'cosmetic',
        'Custom porcelain shells bonded to the front surface of teeth for a perfect smile makeover.',
        8000, 15000,
        'Per tooth.',
        120
    )
ON CONFLICT DO NOTHING;


-- ── Doctors ─────────────────────────────────────────────────────────────────
INSERT INTO clinic_doctors
    (id, clinic_id, name, title, specialization, qualifications,
     bio, experience_years, available_days, consultation_fee, languages)
VALUES
    (
        'c1000001-0000-0000-0000-000000000001',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'Ananya Kapoor', 'Dr.',
        'General & Cosmetic Dentistry',
        '{"BDS","MDS (Oral Medicine)"}',
        'Dr. Ananya has 12 years of experience in smile makeovers, preventive care, and cosmetic dentistry. '
        'She believes in pain-free dentistry and uses the latest laser techniques.',
        12,
        '{"Monday","Tuesday","Wednesday","Thursday","Friday"}',
        400,
        '{"English","Hindi"}'
    ),
    (
        'c1000001-0000-0000-0000-000000000002',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'Rohan Mehta', 'Dr.',
        'Orthodontics',
        '{"BDS","MDS (Orthodontics)","Invisalign Certified"}',
        'Dr. Rohan specialises in braces, clear aligners, and jaw correction. '
        'He has treated over 1,500 orthodontic cases across all age groups.',
        9,
        '{"Tuesday","Thursday","Saturday"}',
        600,
        '{"English","Hindi","Punjabi"}'
    ),
    (
        'c1000001-0000-0000-0000-000000000003',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'Sunita Verma', 'Dr.',
        'Implantology & Oral Surgery',
        '{"BDS","MDS (Oral & Maxillofacial Surgery)","FICOI"}',
        'Dr. Sunita is our implantology expert with fellowship training from the ICOI. '
        'She handles complex extractions, bone grafts, and full-arch implant cases.',
        15,
        '{"Monday","Wednesday","Friday"}',
        800,
        '{"English","Hindi"}'
    )
ON CONFLICT DO NOTHING;


-- ── FAQs ────────────────────────────────────────────────────────────────────
INSERT INTO clinic_faqs (id, clinic_id, category, question, answer)
VALUES
    (
        'd1000001-0000-0000-0000-000000000001',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'appointments',
        'How do I book an appointment?',
        'You can book an appointment by messaging us here on WhatsApp/Telegram, calling our receptionist '
        'Priya at +91-98765-43210, emailing hello@brightsmile.in, or visiting the clinic directly. '
        'We typically confirm appointments within 2 hours.'
    ),
    (
        'd1000001-0000-0000-0000-000000000002',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'appointments',
        'What are your clinic timings?',
        'We are open Monday–Friday 9 AM to 8 PM and Saturday 10 AM to 5 PM. We are closed on Sundays.'
    ),
    (
        'd1000001-0000-0000-0000-000000000003',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'appointments',
        'Can I get a same-day appointment?',
        'Yes, we keep slots open for urgent cases. Call or message us early in the day and we will '
        'do our best to fit you in. Emergency tooth pain is always prioritised.'
    ),
    (
        'd1000001-0000-0000-0000-000000000004',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'pricing',
        'Do you accept dental insurance?',
        'Yes, we work with most major insurance providers including Star Health, Care Health, and '
        'HDFC Ergo. Please carry your insurance card and policy number. We handle the paperwork.'
    ),
    (
        'd1000001-0000-0000-0000-000000000005',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'pricing',
        'What payment methods do you accept?',
        'We accept cash, all major credit/debit cards, UPI (Google Pay, PhonePe, Paytm), and '
        'net banking. We also offer 0% EMI on treatments above ₹10,000 via HDFC and ICICI cards.'
    ),
    (
        'd1000001-0000-0000-0000-000000000006',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'pricing',
        'How much does a dental check-up cost?',
        'A routine check-up and cleaning starts at ₹500 and goes up to ₹1,500 depending on '
        'the level of cleaning required. Digital X-rays are ₹300–₹600 extra if needed.'
    ),
    (
        'd1000001-0000-0000-0000-000000000007',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'general',
        'Is the treatment painful?',
        'We use the latest pain-free techniques including topical anaesthesia before injections, '
        'laser dentistry for soft-tissue procedures, and sedation options for anxious patients. '
        'Most patients are surprised at how comfortable modern dental treatment feels.'
    ),
    (
        'd1000001-0000-0000-0000-000000000008',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'general',
        'Is the clinic hygienic and sterilised?',
        'Absolutely. We follow ISO-certified sterilisation protocols. All instruments are '
        'autoclave-sterilised, and disposable items are used wherever possible. Our operatories '
        'are disinfected between every patient.'
    ),
    (
        'd1000001-0000-0000-0000-000000000009',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'general',
        'Do you treat children?',
        'Yes! We have a dedicated paediatric corner and our team is trained in child-friendly '
        'dentistry. We recommend the first dental visit by age 1, or when the first tooth appears.'
    ),
    (
        'd1000001-0000-0000-0000-000000000010',
        'a1b2c3d4-0000-0000-0000-000000000001',
        'general',
        'Where is the clinic located?',
        'We are at 14, Connaught Place, Block E, New Delhi – 110001. Nearest metro station is '
        'Rajiv Chowk (Blue/Yellow Line), Exit Gate 6. Paid parking available in the CP basement.'
    )
ON CONFLICT DO NOTHING;


-- ── Knowledge chunks (pre-populated, embeddings set to NULL until the        ──
-- ── embedding worker runs; the retriever''s nil-guard skips null vectors)     ──
INSERT INTO clinic_knowledge_chunks
    (clinic_id, source_type, source_id, content, metadata)
SELECT
    'a1b2c3d4-0000-0000-0000-000000000001',
    'faq',
    id,
    'Q: ' || question || E'\nA: ' || answer,
    jsonb_build_object(
        'question', question,
        'answer',   answer,
        'category', category
    )
FROM clinic_faqs
WHERE clinic_id = 'a1b2c3d4-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO clinic_knowledge_chunks
    (clinic_id, source_type, source_id, content, metadata)
SELECT
    'a1b2c3d4-0000-0000-0000-000000000001',
    'service',
    id,
    name || ': ' || COALESCE(description, '') ||
        CASE
            WHEN price_min IS NOT NULL
            THEN E'\nPrice: ₹' || price_min::text ||
                 CASE WHEN price_max != price_min
                      THEN '–₹' || price_max::text
                      ELSE '' END ||
                 COALESCE(' (' || price_note || ')', '')
            ELSE ''
        END,
    jsonb_build_object(
        'name',       name,
        'category',   category,
        'price_min',  price_min,
        'price_max',  price_max,
        'price_note', price_note,
        'duration_minutes', duration_minutes
    )
FROM clinic_services
WHERE clinic_id = 'a1b2c3d4-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

INSERT INTO clinic_knowledge_chunks
    (clinic_id, source_type, source_id, content, metadata)
SELECT
    'a1b2c3d4-0000-0000-0000-000000000001',
    'doctor',
    id,
    title || ' ' || name || ', ' || COALESCE(specialization, '') || '.' ||
        COALESCE(E'\n' || bio, '') ||
        E'\nAvailable: ' || array_to_string(available_days, ', ') ||
        E'\nConsultation fee: ₹' || COALESCE(consultation_fee::text, 'TBD'),
    jsonb_build_object(
        'name',              name,
        'title',             title,
        'specialization',    specialization,
        'qualifications',    qualifications,
        'available_days',    available_days,
        'consultation_fee',  consultation_fee,
        'languages',         languages
    )
FROM clinic_doctors
WHERE clinic_id = 'a1b2c3d4-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

COMMIT;
