def handler(params, context):
    try:
        text = str(params["text"])
        shift = int(params["shift"])
        result = ""
        for i in range(len(text)):
            char = text[i]
            if char.isupper():
                result += chr((ord(char) + shift - 65) % 26 + 65)
            else:
                result += chr((ord(char) + shift - 97) % 26 + 97)
        return {"Result": result}
    except:
        return {}