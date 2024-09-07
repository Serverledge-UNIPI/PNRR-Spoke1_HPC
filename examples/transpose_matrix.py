def handler(params, context):
    try:
        matrix = params["matrix"]
        result = [list(row) for row in zip(*matrix)]
        return { "Transposed matrix": matrix}
    except:
        return {}