def handler(params, context):
    try:
        array = params["array"]
        x = params["x"]

        result = -1
        for i in range(len(array)):
            if array[i] == x:
                result = i
                break
        return {"Result": result}
    except:
        return {}
