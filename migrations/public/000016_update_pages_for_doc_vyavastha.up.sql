-- Remove old project pages
DELETE FROM public.pages 
WHERE route_url IN ('/fee-plans', '/payments', '/daily-collections');

-- Insert new project pages
INSERT INTO public.pages (page_name, route_url, display_order)
SELECT v.page_name, v.route_url, CAST(v.display_order AS INT)
FROM (VALUES
    ('Clients', '/dashboard/clients', 2),
    ('Organizations', '/dashboard/client-organizations', 3),
    ('Applications', '/dashboard/applications', 4),
    ('Document Vault', '/dashboard/document-vault', 5)
) AS v(page_name, route_url, display_order)
WHERE NOT EXISTS (
    SELECT 1 FROM public.pages p WHERE p.route_url = v.route_url
);

-- Grant Admin full access to these new pages
INSERT INTO public.role_page_permissions (role_id, page_id, can_view, can_add, can_edit, can_delete)
SELECT r.role_id, p.page_id, true, true, true, true
FROM public.roles r
CROSS JOIN public.pages p
WHERE r.role_name = 'Admin' AND p.route_url IN (
    '/dashboard/clients', 
    '/dashboard/client-organizations',
    '/dashboard/applications',
    '/dashboard/document-vault'
)
ON CONFLICT (role_id, page_id) DO NOTHING;
