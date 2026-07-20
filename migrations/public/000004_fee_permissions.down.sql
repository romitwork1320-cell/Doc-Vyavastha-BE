-- Remove Fee Plans and Payments pages

DELETE FROM public.pages WHERE route_url IN ('/fee-plans', '/payments');
