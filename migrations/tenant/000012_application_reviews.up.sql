CREATE TABLE application_requirement_reviews (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    requirement_id BIGINT NOT NULL REFERENCES application_requirements(id) ON DELETE CASCADE,
    reviewer_id BIGINT,
    reviewer_type VARCHAR(50) NOT NULL, -- 'ORGANIZATION', 'CLIENT'
    reviewer_name VARCHAR(255),
    status VARCHAR(50) NOT NULL,
    comment TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_application_req_reviews_req ON application_requirement_reviews(requirement_id);
