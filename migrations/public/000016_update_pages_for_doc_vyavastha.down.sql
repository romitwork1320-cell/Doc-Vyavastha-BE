-- Remove new project pages
DELETE FROM public.pages 
WHERE route_url IN (
    '/dashboard/clients', 
    '/dashboard/client-organizations',
    '/dashboard/applications',
    '/dashboard/document-vault'
);

-- Re-insert old project pages
INSERT INTO public.pages (page_name, route_url, display_order)
SELECT v.page_name, v.route_url, CAST(v.display_order AS INT)
FROM (VALUES
    ('Fee Plans', '/fee-plans', 15),
    ('Payments', '/payments', 16),
    ('Daily Collections', '/daily-collections', 17)
) AS v(page_name, route_url, display_order)
WHERE NOT EXISTS (
    SELECT 1 FROM public.pages p WHERE p.route_url = v.route_url
);
