-- Add Fee Plans and Payments as separate pages in the system

INSERT INTO public.pages (page_name, route_url, display_order)
SELECT 'Fee Plans', '/fee-plans', 15
WHERE NOT EXISTS (SELECT 1 FROM public.pages WHERE route_url = '/fee-plans');

INSERT INTO public.pages (page_name, route_url, display_order)
SELECT 'Payments', '/payments', 16
WHERE NOT EXISTS (SELECT 1 FROM public.pages WHERE route_url = '/payments');

-- Grant Admin full access to these new pages
INSERT INTO public.role_page_permissions (role_id, page_id, can_view, can_add, can_edit, can_delete)
SELECT r.role_id, p.page_id, true, true, true, true
FROM public.roles r
CROSS JOIN public.pages p
WHERE r.role_name = 'Admin' AND p.route_url IN ('/fee-plans', '/payments')
ON CONFLICT (role_id, page_id) DO NOTHING;
