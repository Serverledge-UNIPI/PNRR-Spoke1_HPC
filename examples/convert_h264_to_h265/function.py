import ffmpeg

input_path = '/home/ubuntu/BigBuckBunny_320x180.mp4'
output_path = '/home/ubuntu/BigBuckBunny_320x180_h265.mp4'

def handler(params, context):
    try:
        ffmpeg.input(input_path).output(output_path, vcodec='libx265').run()
        result = {"Result": "Conversion successful!"}
    except ffmpeg._run.Error as e:
        result = {"Error": str(e.stderr.decode())}
    return result