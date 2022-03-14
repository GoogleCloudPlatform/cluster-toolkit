def custom_error_403(request, exception):
    return render(request, '403.html', {})
