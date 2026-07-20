-- Insert Daily Collections into public.pages
INSERT INTO public.pages (page_name, route_url, display_order)
SELECT 'Daily Collections', '/daily-collections', 20
WHERE NOT EXISTS (SELECT 1 FROM public.pages WHERE route_url = '/daily-collections');

-- Grant full access to Admin role
INSERT INTO public.role_page_permissions (role_id, page_id, can_view, can_add, can_edit, can_delete)
SELECT r.role_id, p.page_id, true, true, true, true
FROM public.roles r
CROSS JOIN public.pages p
WHERE r.role_name = 'Admin' AND p.route_url = '/daily-collections'
ON CONFLICT (role_id, page_id) DO NOTHING;

-- Grant partial access to Staff role
INSERT INTO public.role_page_permissions (role_id, page_id, can_view, can_add, can_edit, can_delete)
SELECT r.role_id, p.page_id, true, true, false, false
FROM public.roles r
CROSS JOIN public.pages p
WHERE r.role_name = 'Staff' AND p.route_url = '/daily-collections'
ON CONFLICT (role_id, page_id) DO NOTHING;
