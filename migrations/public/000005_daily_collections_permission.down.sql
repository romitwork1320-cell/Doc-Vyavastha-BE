DELETE FROM public.role_page_permissions
WHERE page_id IN (SELECT page_id FROM public.pages WHERE route_url = '/daily-collections');

DELETE FROM public.pages WHERE route_url = '/daily-collections';
