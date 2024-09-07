def handler(params, context):
    try:
        a = params["a"]
        b = params["b"]
        while b:
            a, b = b, a % b
        return a
    except:
        return {}